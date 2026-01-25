package sliding_window

import (
	"math"
	"sort"
)

const (
	CryptoDefaultAlpha = 0.7
	CryptoDefaultBeta  = 0.15
)

type WindowStats struct {
	// 核心整数统计（稳定）
	HighTicks   float64
	LowTicks    float64
	NewestTicks float64
	OldestTicks float64

	SumPV float64 // Σ(priceTicks * volUnits)
	SumV  float64 // Σ(volUnits)

	// 用于 median（也用 ticks，避免 float 排序误差）
	Prices []float64
}

func (w *SlidingWindow) collectStats(prices []float64) (WindowStats, bool) {
	var stats WindowStats

	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.size < 2 {
		return stats, false
	}
	if len(prices) < w.size {
		return stats, false
	}

	n := w.size
	stats.Prices = prices[:n] // ✅ 关键：把 stats.Prices 指向外部 buffer

	first := w.atUnlocked(0)
	hi := first.Price.Float(w.priceScale)
	lo := hi

	oldest := first
	newest := w.lastUnlocked()

	stats.OldestTicks = oldest.Price.Float(w.priceScale)
	stats.NewestTicks = newest.Price.Float(w.priceScale)

	for i := 0; i < n; i++ {
		pt := w.atUnlocked(i)
		px := pt.Price.Float(w.priceScale)
		v := pt.Volume.Float(w.volumeScale)

		prices[i] = px

		if px > hi {
			hi = px
		}
		if px < lo {
			lo = px
		}

		stats.SumPV += px * v
		stats.SumV += v
	}

	stats.HighTicks = hi
	stats.LowTicks = lo
	return stats, true
}

func (w *SlidingWindow) EquilibriumZone(alpha, beta float64) (EquilibriumZone, bool) {
	var empty EquilibriumZone

	w.mu.RLock()
	if w.size < 2 {
		w.mu.RUnlock()
		return empty, false
	}

	n := w.size
	prices, pb := w.getPricesBuf(n)

	first := w.atUnlocked(0)
	high := first.Price.Float(w.priceScale)
	low := high

	oldest := first.Price.Float(w.priceScale)
	newest := w.lastUnlocked().Price.Float(w.priceScale)

	var sumPV, sumV float64

	for i := 0; i < n; i++ {
		pt := w.atUnlocked(i)
		px := pt.Price.Float(w.priceScale)
		v := pt.Volume.Float(w.volumeScale)

		prices[i] = px

		if px > high {
			high = px
		}
		if px < low {
			low = px
		}

		sumPV += px * v
		sumV += v
	}
	w.mu.RUnlock()

	// ====== 从这里开始，所有 return 前都要 put ======
	if sumV <= 0 {
		w.putPricesBuf(pb)
		return empty, false
	}

	vwap := sumPV / sumV

	sort.Float64s(prices)

	var median float64
	if n%2 == 1 {
		median = prices[n/2]
	} else {
		median = (prices[n/2-1] + prices[n/2]) / 2
	}

	equ := alpha*vwap + (1-alpha)*median

	rng := high - low
	if rng <= 0 || oldest == 0 {
		w.putPricesBuf(pb)
		return empty, false
	}

	ret := (newest - oldest) / oldest
	retScale := math.Abs(ret) * newest

	bw := beta * rng
	if bw < retScale {
		bw = retScale
	}
	if bw <= 1e-12 {
		w.putPricesBuf(pb)
		return empty, false
	}

	dist := newest - equ
	zone := EquilibriumZone{
		EquPrice:  equ,
		UpperBand: equ + bw,
		LowerBand: equ - bw,
		BandWidth: bw,
		Price:     newest,
		Distance:  dist,
		NormDist:  dist / bw,
	}

	w.putPricesBuf(pb)
	return zone, true
}

type EquilibriumZone struct {
	EquPrice  float64 `json:"equ_price"`
	UpperBand float64 `json:"upper_band"`
	LowerBand float64 `json:"lower_band"`
	BandWidth float64 `json:"band_width"`
	Price     float64 `json:"price"`
	Distance  float64 `json:"distance"`
	NormDist  float64 `json:"norm_dist"`
}

func (w *SlidingWindow) getPricesBuf(n int) ([]float64, *pricesBuf) {
	p := w.pricesPool.Get().(*pricesBuf)

	if cap(p.b) < n {
		// 指数扩容：减少后续 make 次数
		newCap := cap(p.b)
		if newCap < 256 {
			newCap = 256
		}
		for newCap < n {
			newCap *= 2
		}
		p.b = make([]float64, n, newCap) // len=n, cap=newCap
	} else {
		p.b = p.b[:n]
	}

	return p.b, p
}

func (w *SlidingWindow) putPricesBuf(p *pricesBuf) {
	// 防止 pool 被极端大窗口撑爆内存
	const maxCap = 1 << 16 // 65536，可按你窗口上限调整
	if cap(p.b) > maxCap {
		return
	}
	p.b = p.b[:0]
	w.pricesPool.Put(p)
}
