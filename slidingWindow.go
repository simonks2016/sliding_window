package sliding_window

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"
)

type SlidingWindow struct {
	duration  time.Duration // 窗口长度，比如 60 * time.Second
	buf       []WindowPoint // 环形数组
	start     int           // 头指针
	size      int           // 当前有效元素个数
	sumVolume float64       // 窗口内成交量总和
	mu        sync.RWMutex  // 并发安全
	ema       *EMA
}

func NewSlidingWindow(duration time.Duration, capacity int, emaAlpha float64) *SlidingWindow {
	return &SlidingWindow{
		duration: duration,
		buf:      make([]WindowPoint, capacity),
		ema:      NewEMA(emaAlpha),
	}
}

func (w *SlidingWindow) atUnlocked(i int) WindowPoint {
	// i assumed in [0, w.size)
	return w.buf[(w.start+i)%len(w.buf)]
}

func (w *SlidingWindow) lastUnlocked() WindowPoint {
	return w.atUnlocked(w.size - 1)
}

// --- 公共方法（带锁） ---
func (w *SlidingWindow) at(i int) WindowPoint {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.atUnlocked(i)
}

func (w *SlidingWindow) last() WindowPoint {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.lastUnlocked()
}

// Add 添加一个点并自动清理超出时间窗口的旧点（写锁）
func (w *SlidingWindow) Add(p WindowPoint) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.size == 0 {
		w.buf[0] = p
		w.start = 0
		w.size = 1
		w.sumVolume = p.Volume
		return
	}

	if w.size < len(w.buf) {
		idx := (w.start + w.size) % len(w.buf)
		w.buf[idx] = p
		w.size++
	} else {
		// 覆盖头部（环形）
		idx := (w.start + w.size) % len(w.buf) // 等于 w.start when size==len(buf)
		old := w.buf[idx]
		w.sumVolume -= old.Volume

		w.buf[idx] = p
		w.start = (w.start + 1) % len(w.buf)
	}
	w.sumVolume += p.Volume

	// 根据时间戳滑动窗口（移除不在窗口内的旧点）
	threshold := p.Ts.Add(-w.duration)
	for w.size > 0 {
		head := w.atUnlocked(0)
		// 保持 head 在 (threshold, +inf] 才算有效
		if head.Ts.After(threshold) {
			break
		}
		w.sumVolume -= head.Volume
		w.start = (w.start + 1) % len(w.buf)
		w.size--
	}
	if p.Volume > 0 {
		w.ema.Update(p.Volume)
	}
}

// Ready 真实（读锁）
func (w *SlidingWindow) Ready(minPoints int) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.size >= minPoints
}

// Snapshot 快照（读锁）返回窗口首尾价格与总量
func (w *SlidingWindow) Snapshot() (pOld, pNew, volSum float64, ok bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.size < 2 {
		return 0, 0, 0, false
	}
	old := w.atUnlocked(0)
	newest := w.lastUnlocked()
	return old.Price, newest.Price, w.sumVolume, true
}

// SumVolume 返回当前窗口内成交量总和（读锁）
func (w *SlidingWindow) SumVolume() float64 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.sumVolume
}

// 无锁版计算交易量基准
func (w *SlidingWindow) volumeFactor() (float64, bool) {

	baselineVol, ok := w.ema.Get()
	if !ok {
		return 0, false
	}

	if w.size == 0 || baselineVol <= 0 {
		return 0, false
	}

	currAvg := w.sumVolume / float64(w.size) // 当前窗口平均每笔/每点成交量
	if currAvg <= 0 {
		return 0, false
	}

	vf := currAvg / baselineVol
	if vf < 0 {
		return 0, false
	}
	return vf, true

}

// VolumeFactor 带锁计算交易量基准
func (w *SlidingWindow) VolumeFactor() (float64, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return w.volumeFactor()
}

// Momentum 计算简单“价格 + 量能”动能因子 avgVolume 建议用 EMA.Value 作为参考平均成交量
func (w *SlidingWindow) Momentum() (momentum float64, ok bool) {

	w.mu.RLock()
	defer w.mu.RUnlock()

	VolFactor, ok := w.volumeFactor()
	if !ok || w.size < 2 {
		return 0, false
	}

	old := w.atUnlocked(0)
	newest := w.lastUnlocked()

	// 价格收益率
	if old.Price == 0 {
		return 0, false
	}
	ret := (newest.Price - old.Price) / old.Price

	// 动能 = 收益率 * log(1 + volFactor)
	m := ret * math.Log1p(VolFactor)

	return m, true
}

// ClassifyMomentum 根据阈值分级
func (w *SlidingWindow) ClassifyMomentum(avgVolume, weak, strong float64) (MomentumSignal, bool) {
	var empty MomentumSignal
	if avgVolume <= 0 {
		return empty, false
	}

	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.size < 2 {
		return empty, false
	}

	old := w.atUnlocked(0)
	newest := w.lastUnlocked()
	if old.Price == 0 {
		return empty, false
	}
	ret := (newest.Price - old.Price) / old.Price
	volFactor := w.sumVolume / avgVolume
	if volFactor < 0 {
		volFactor = 0
	}
	val := ret * math.Log1p(volFactor)

	level := MomentumNeutral
	absVal := math.Abs(val)

	if absVal >= strong {
		if val > 0 {
			level = MomentumStrongUp
		} else {
			level = MomentumStrongDown
		}
	} else if absVal >= weak {
		if val > 0 {
			level = MomentumUp
		} else {
			level = MomentumDown
		}
	} else {
		level = MomentumNeutral
	}

	return MomentumSignal{
		Level:     level,
		Value:     val,
		Ret:       ret,
		VolFactor: volFactor,
	}, true
}

// Window 内总成交量
func (w *SlidingWindow) TotalVolume() float64 {
	return w.SumVolume()
}

// AvgVolumePerPoint Window 内每个点的平均成交量（不是时间归一化的）
func (w *SlidingWindow) AvgVolumePerPoint() float64 {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.size == 0 {
		return 0
	}
	return w.sumVolume / float64(w.size)
}

// VolumePerSecond 按时间归一化的成交量（每秒多少量）
func (w *SlidingWindow) VolumePerSecond() float64 {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.size == 0 {
		return 0
	}
	old := w.atUnlocked(0)
	newest := w.lastUnlocked()

	sec := newest.Ts.Sub(old.Ts).Seconds()
	if sec <= 0 {
		return 0
	}
	return w.sumVolume / sec
}

func (w *SlidingWindow) MedianPrice() (median float64, ok bool) {
	w.mu.RLock()
	// 复制价格到局部 slice（避免锁内做过多工作，但仍要保护 buf）
	if w.size == 0 {
		w.mu.RUnlock()
		return 0, false
	}

	prices := make([]float64, w.size)
	for i := 0; i < w.size; i++ {
		prices[i] = w.atUnlocked(i).Price
	}
	w.mu.RUnlock()

	// 排序与计算可以在没有锁时进行（我们已经把值复制出来）
	sort.Float64s(prices)

	n := len(prices)
	if n%2 == 1 {
		return prices[n/2], true
	}
	mid1 := prices[n/2-1]
	mid2 := prices[n/2]
	return (mid1 + mid2) / 2.0, true
}

// VolumeWeightedAveragePrice
func (w *SlidingWindow) VolumeWeightedAveragePrice() (float64, bool) {
	w.mu.RLock()
	if w.size == 0 {
		w.mu.RUnlock()
		return 0, false
	}

	var sumPV, sumV float64
	for i := 0; i < w.size; i++ {
		p := w.atUnlocked(i)
		sumPV += p.Price * p.Volume
		sumV += p.Volume
	}
	w.mu.RUnlock()

	if sumV <= 0 {
		return 0, false
	}
	return sumPV / sumV, true
}

// ScoreWithMomentum 计算价格趋势 + 动量 + 订单流贝叶斯置信后的综合得分。
// currentMomentum: 当前动量因子
// dirScale: 用于归一化方向收益率，比如 0.005 表示 0.5% 涨跌映射到 ±1。
// momentumScale: 用于归一化动量值。
// orderFlowConfidence: 订单流置信因子，约定在 [-1,1]
func (w *SlidingWindow) ScoreWithMomentum(currentMomentum, dirScale, momentumScale, orderFlowConfidence float64) (float64, error) {
	if dirScale <= 1e-6 || momentumScale <= 1e-6 {
		return 0, fmt.Errorf("the dir scale or momentum scale is zero,%.2f,%.2f\n", dirScale, momentumScale)
	}

	// 为保证一致性，需要在同一个读锁内读取 Snapshot 和 Momentum 所需字段
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.size < 2 {
		return 0, fmt.Errorf("the momentum size is too small,%d\n", w.size)
	}
	pOld := w.atUnlocked(0).Price
	pNew := w.lastUnlocked().Price

	// 价格侧方向
	side := (pNew - pOld)
	if pOld != 0 {
		side = (pNew - pOld) / pOld
	} else {
		// 避免除以0
		side = 0
	}
	// 计算价格方向
	dirFactor := side / dirScale
	if dirFactor > 1 {
		dirFactor = 1
	} else if dirFactor < -1 {
		dirFactor = -1
	}
	// 引入外部动量
	momFactor := currentMomentum / momentumScale
	if momFactor > 1 {
		momFactor = 1
	} else if momFactor < -1 {
		momFactor = -1
	}

	trendFactor := 0.5*dirFactor + 0.5*momFactor
	if math.Abs(trendFactor) < 1e-8 {
		return 0, nil
	}

	if orderFlowConfidence > 1 {
		orderFlowConfidence = 1
	} else if orderFlowConfidence < -1 {
		orderFlowConfidence = -1
	}

	trendSign := 1.0
	if trendFactor < 0 {
		trendSign = -1.0
	}
	strength := math.Abs(trendFactor)

	confWeight := (1 + trendSign*orderFlowConfidence) / 2
	if confWeight < 0 {
		confWeight = 0
	} else if confWeight > 1 {
		confWeight = 1
	}

	score := trendSign * strength * confWeight
	return score, nil
}

const (
	defaultDirScale      = 0.05
	defaultMomentumScale = 0.1
)
