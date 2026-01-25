package sliding_window

import (
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

type SlidingWindow struct {
	duration       time.Duration // 窗口长度，比如 60 * time.Second
	buf            []WindowPoint // 环形数组
	pricesPool     sync.Pool
	start          int          // 头指针
	size           int          // 当前有效元素个数
	sumVolume      QtyLoz // 窗口内成交量总和
	mu             sync.RWMutex // 并发安全
	ema            *EMA
	volumeScale    QtyScale
	priceScale     QtyScale
	avgVolPerPoint atomic.Int64
	volPerSecond   atomic.Int64
	buyVol         atomic.Int64
	sellVol        atomic.Int64
	nTrades        atomic.Int64
	HighestPrice   atomic.Int64
	LowestPrice    atomic.Int64
	LatestPrice    atomic.Int64
	SumV           atomic.Int64
	SumPV          atomic.Int64
	hiLoDirty      bool
}

type pricesBuf struct {
	b []float64
}

func NewSlidingWindow(duration time.Duration, capacity int, emaAlpha float64) *SlidingWindow {
	w := &SlidingWindow{
		duration:    duration,
		buf:         make([]WindowPoint, capacity),
		ema:         NewEMA(emaAlpha),
		volumeScale: NewQtyScaleFromDecimals(8),
		priceScale:  NewQtyScaleFromDecimals(4),
	}

	w.pricesPool.New = func() any {
		return &pricesBuf{b: make([]float64, 0, 256)}
	}

	return w
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

// Ready 真实（读锁）
func (w *SlidingWindow) Ready(minPoints int) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.size >= minPoints
}

// SumVolume 返回当前窗口内成交量总和（读锁）
func (w *SlidingWindow) SumVolume() float64 {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return w.sumVolume.Float(w.volumeScale)
}

// AvgVolumePerPoint Window 内每个点的平均成交量（不是时间归一化的）
func (w *SlidingWindow) AvgVolumePerPoint() float64 {
	p1 := w.avgVolPerPoint.Load()
	return QtyLoz(p1).Float(w.volumeScale)
}

// VolumePerSecond 按时间归一化的成交量（每秒多少量）
func (w *SlidingWindow) VolumePerSecond() float64 {
	p1 := w.volPerSecond.Load()
	return QtyLoz(p1).Float(w.volumeScale)
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
	side := pNew.Float(w.priceScale) - pOld.Float(w.priceScale)
	if !pOld.IsZero() {
		side = (pNew.Float(w.priceScale) - pOld.Float(w.priceScale)) / pOld.Float(w.priceScale)
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
