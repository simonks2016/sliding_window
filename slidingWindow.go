package sliding_window

import (
	"math"
	"sort"
	"time"
)

type SlidingWindow struct {
	duration  time.Duration // 窗口长度，比如 60 * time.Second
	buf       []WindowPoint // 环形数组
	start     int           // 头指针
	size      int           // 当前有效元素个数
	sumVolume float64       // 窗口内成交量总和
}

func NewSlidingWindow(duration time.Duration, capacity int) *SlidingWindow {
	return &SlidingWindow{
		duration: duration,
		buf:      make([]WindowPoint, capacity),
	}
}

func (w *SlidingWindow) at(i int) WindowPoint {
	return w.buf[(w.start+i)%len(w.buf)]
}

func (w *SlidingWindow) last() WindowPoint {
	return w.at(w.size - 1)
}

// Add 添加一个点并自动清理超出时间窗口的旧点
func (w *SlidingWindow) Add(p WindowPoint) {
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
		idx := (w.start + w.size) % len(w.buf)
		old := w.buf[idx]
		w.sumVolume -= old.Volume

		w.buf[idx] = p
		w.start = (w.start + 1) % len(w.buf)
	}
	w.sumVolume += p.Volume

	// 根据时间戳滑动窗口
	threshold := p.Ts.Add(-w.duration)
	for w.size > 0 {
		head := w.at(0)
		if head.Ts.After(threshold) {
			break
		}
		w.sumVolume -= head.Volume
		w.start = (w.start + 1) % len(w.buf)
		w.size--
	}
}

// Ready 真实
func (w *SlidingWindow) Ready(minPoints int) bool {
	return w.size >= minPoints
}

// Snapshot 快照
func (w *SlidingWindow) Snapshot() (pOld, pNew, volSum float64, ok bool) {
	if w.size < 2 {
		return 0, 0, 0, false
	}
	old := w.at(0)
	newest := w.last()
	return old.Price, newest.Price, w.sumVolume, true
}

// SumVolume 返回当前窗口内成交量总和
func (w *SlidingWindow) SumVolume() float64 {
	return w.sumVolume
}

// Momentum 计算简单“价格 + 量能”动能因子 avgVolume 建议用 EMA.Value 作为参考平均成交量
func (w *SlidingWindow) Momentum(avgVolume float64) (momentum float64, ok bool) {
	if w.size < 2 || avgVolume <= 0 {
		return 0, false
	}
	old := w.at(0)
	newest := w.last()

	// 价格收益率
	ret := (newest.Price - old.Price) / old.Price

	// 成交量放大倍数
	volFactor := w.sumVolume / avgVolume
	if volFactor < 0 {
		volFactor = 0
	}

	// 动能 = 收益率 * log(1 + volFactor)
	m := ret * math.Log1p(volFactor)

	return m, true
}

// ClassifyMomentum 根据阈值分级
// weak 是普通趋势阈值，strong 是强趋势阈值
// 比如 weak=0.001（0.1%），strong=0.003（0.3%）
func (w *SlidingWindow) ClassifyMomentum(avgVolume, weak, strong float64) (MomentumSignal, bool) {
	var empty MomentumSignal
	if w.size < 2 || avgVolume <= 0 {
		return empty, false
	}

	old := w.at(0)
	newest := w.last()
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
	return w.sumVolume
}

// AvgVolumePerPoint Window 内每个点的平均成交量（不是时间归一化的）
func (w *SlidingWindow) AvgVolumePerPoint() float64 {
	if w.size == 0 {
		return 0
	}
	return w.sumVolume / float64(w.size)
}

// VolumePerSecond 按时间归一化的成交量（每秒多少量）
func (w *SlidingWindow) VolumePerSecond() float64 {
	if w.size == 0 {
		return 0
	}
	old := w.at(0)
	newest := w.last()

	sec := newest.Ts.Sub(old.Ts).Seconds()
	if sec <= 0 {
		return 0
	}
	return w.sumVolume / sec
}

func (w *SlidingWindow) MedianPrice() (median float64, ok bool) {
	if w.size == 0 {
		return 0, false
	}

	// 收集当前窗口内所有价格
	prices := make([]float64, w.size)
	for i := 0; i < w.size; i++ {
		prices[i] = w.at(i).Price
	}

	// 排序
	sort.Float64s(prices)

	n := len(prices)
	if n%2 == 1 {
		// 奇数个：取中间那个
		return prices[n/2], true
	}

	// 偶数个：取中间两个的平均
	mid1 := prices[n/2-1]
	mid2 := prices[n/2]
	return (mid1 + mid2) / 2.0, true
}
