package sliding_window

import (
	"math"
	"sort"
)

type ADKind int

const (
	ADNeutral      ADKind = iota
	ADAbsorption          // 吸筹
	ADDistribution        // 派发
)

type AbsorptionSignal struct {
	Kind            ADKind
	Score           float64 // 正=吸筹，负=派发，绝对值越大越明显
	Ret             float64
	VolumeFactor    float64
	VWAP            float64
	Median          float64
	VwapMinusMedian float64
}

// AbsorptionDistribution 用“成交分布偏移 + 量能放大 + 价格方向”识别吸筹/派发
/*思路：
1) VWAP - Median：衡量成交重心偏移（偏上可能是买盘主动/被动堆积）
2) VolumeFactor：量能放大才“可信”
3) Ret：收益率太大时往往是趋势性拉升/下杀，而不是“吸筹/派发”
4) 用阈值控制：
	- minVF: 量能至少放大多少才判定
  	- maxAbsRet: 收益率绝对值不应太大（太大更像趋势突破而不是吸筹/派发）
	- scoreStrong/scoreWeak: 分类阈值
*/
func (w *SlidingWindow) AbsorptionDistribution(
	minVF, maxAbsRet, scoreWeak, scoreStrong float64,
) (AbsorptionSignal, bool) {
	var empty AbsorptionSignal

	w.mu.RLock()
	if w.size < 2 {
		w.mu.RUnlock()
		return empty, false
	}

	n := w.size
	prices, pb := w.getPricesBuf(n)

	// oldest/newest（用于 ret）
	oldestPx := w.atUnlocked(0).Price.Float(w.priceScale)
	newestPx := w.lastUnlocked().Price.Float(w.priceScale)

	// === 量能因子 vf：优先走“锁内版本”，避免 VolumeFactor() 里再加锁 ===
	// 你如果没有 volumeFactorUnlocked，就先用 VolumeFactor()，但会多一次锁。
	vf, ok := w.volumeFactor() // ✅建议你提供这个
	if !ok {
		w.mu.RUnlock()
		w.putPricesBuf(pb)
		return empty, false
	}

	// === VWAP：锁内累加 sumPV/sumV，同时填 prices[] 给 median 用 ===
	var sumPV, sumV float64
	for i := 0; i < n; i++ {
		pt := w.atUnlocked(i)
		px := pt.Price.Float(w.priceScale)
		v := pt.Volume.Float(w.volumeScale)

		prices[i] = px
		sumPV += px * v
		sumV += v
	}
	w.mu.RUnlock()

	// ===== 锁外：开始做判断/排序 =====
	defer w.putPricesBuf(pb)

	if oldestPx == 0 || sumV <= 0 {
		return empty, false
	}

	ret := (newestPx - oldestPx) / oldestPx
	if math.Abs(ret) > maxAbsRet {
		// 太像趋势行情，吸筹/派发意义不大
		return empty, false
	}

	if vf < minVF {
		return empty, false
	}

	vwap := sumPV / sumV

	// median：直接 sort(prices)，因为 prices 来自 pool，本来就不是共享数据
	sort.Float64s(prices)
	var median float64
	if n%2 == 1 {
		median = prices[n/2]
	} else {
		median = (prices[n/2-1] + prices[n/2]) / 2.0
	}

	diff := vwap - median

	scale := math.Abs(median)
	if scale <= 1e-12 {
		return empty, false
	}
	diffNorm := diff / scale

	sideways := 1.0 - math.Min(1.0, math.Abs(ret)/maxAbsRet)
	score := diffNorm * math.Log1p(vf) * sideways

	kind := ADNeutral
	absScore := math.Abs(score)
	if absScore >= scoreWeak {
		if score > 0 {
			kind = ADAbsorption
		} else {
			kind = ADDistribution
		}
	}
	if absScore >= scoreStrong {
		// 强信号仍然是同一方向，只是强度更高
		if score > 0 {
			kind = ADAbsorption
		} else {
			kind = ADDistribution
		}
	}

	return AbsorptionSignal{
		Kind:            kind,
		Score:           score,
		Ret:             ret,
		VolumeFactor:    vf,
		VWAP:            vwap,
		Median:          median,
		VwapMinusMedian: diff,
	}, true
}
