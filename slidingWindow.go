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

// VolumeWeightedAveragePrice
// 交易量加权平均交易价格,这个是获取一定时间内根据交易量所得出交易平均价格
func (w *SlidingWindow) VolumeWeightedAveragePrice() (float64, bool) {
	if w.size == 0 {
		return 0, false
	}
	var sumPV, sumV float64
	for i := 0; i < w.size; i++ {
		p := w.at(i)
		sumPV += p.Price * p.Volume
		sumV += p.Volume
	}
	if sumV <= 0 {
		return 0, false
	}
	return sumPV / sumV, true
}

// Score 计算价格趋势 + 动量 + 订单流贝叶斯置信后的综合得分。
// dirScale: 用于归一化方向收益率，比如 0.005 表示 0.5% 涨跌映射到 ±1。
// momentumScale: 用于归一化动量值。
// orderFlowConfidence: 订单流置信因子，约定在 [-1,1]：
//
//	>0 偏多，<0 偏空，0 附近表示中性。
func (w *SlidingWindow) Score(dirScale, momentumScale, orderFlowConfidence float64) (float64, bool) {
	if dirScale <= 0 || momentumScale <= 0 {
		return 0, false
	}

	pOld, pNew, _, ok := w.Snapshot()
	if !ok {
		return 0, false
	}

	// 1. 方向因子：价格从窗口头到尾的收益率
	side := (pNew - pOld) / pOld
	dirFactor := side / dirScale
	if dirFactor > 1 {
		dirFactor = 1
	} else if dirFactor < -1 {
		dirFactor = -1
	}

	// 2. 动量因子：你的 Momentum 指标（已经包含价格+成交量信息）
	mom, ok := w.Momentum(w.AvgVolumePerPoint())
	if !ok {
		return 0, false
	}
	momFactor := mom / momentumScale
	if momFactor > 1 {
		momFactor = 1
	} else if momFactor < -1 {
		momFactor = -1
	}

	// 3. 先把“价格侧趋势”合成一个因子（方向 + 力度）
	trendFactor := 0.5*dirFactor + 0.5*momFactor // [-1,1]
	if math.Abs(trendFactor) < 1e-8 {
		// 没有明显趋势，就算订单流有偏向，也不轻易给方向
		return 0, true
	}

	// 4. 订单流置信因子裁剪到 [-1,1]
	if orderFlowConfidence > 1 {
		orderFlowConfidence = 1
	} else if orderFlowConfidence < -1 {
		orderFlowConfidence = -1
	}

	// 5. “贝叶斯”思想：
	//    - trendFactor 是“先验方向”：>0 看多，<0 看空
	//    - orderFlowConfidence 是“似然”：资金是否支持这方向
	//    → 我们用它来调节“可信度权重”
	trendSign := 1.0
	if trendFactor < 0 {
		trendSign = -1.0
	}
	strength := math.Abs(trendFactor) // 趋势力度 [0,1]

	// 6. 置信权重（0~1）：
	//    confWeight = (1 + trendSign * orderFlowConfidence) / 2
	//    情况：
	//      - 趋势向上(trendSign=+1)，订单流也偏多(orderFlowConf>0) → 权重 > 0.5，最高到 1
	//      - 趋势向上，订单流偏空(orderFlowConf<0) → 权重 < 0.5，极端时到 0（完全不信）
	//      - 趋势向下(trendSign=-1)，订单流偏空(orderFlowConf<0) → 权重 > 0.5
	//      - 趋势向下，订单流偏多(orderFlowConf>0) → 权重接近 0
	confWeight := (1 + trendSign*orderFlowConfidence) / 2
	if confWeight < 0 {
		confWeight = 0
	} else if confWeight > 1 {
		confWeight = 1
	}

	// 7. 最终得分：
	//    - 符号只由“价格趋势方向”决定（trendSign）
	//    - 绝对值 = 趋势力度 * 订单流可信度
	score := trendSign * strength * confWeight

	return score, true
}

const (
	defaultDirScale      = 0.05
	defaultMomentumScale = 0.1
)
