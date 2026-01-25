package sliding_window

import (
	"math"
)

// Momentum 计算简单“价格 + 量能”动能因子 avgVolume 建议用 EMA.Value 作为参考平均成交量
func (w *SlidingWindow) Momentum() (momentum float64, ok bool) {

	w.mu.RLock()
	vf, ok1 := w.volumeFactor()
	ret, ok2 := w.structuralReturn()
	w.mu.RUnlock()

	if ok1 && ok2 {
		// 动能 = 收益率 * log(1 + volFactor)
		m := ret * math.Log1p(vf)

		return m, true
	}

	return momentum, false
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

	// 计算标准收益率
	ret, ok := w.structuralReturn()
	if !ok {
		return empty, false
	}

	volFactor := w.sumVolume.Float(w.volumeScale) / avgVolume
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

type MomentumLevel int

const (
	MomentumStrongDown MomentumLevel = -2
	MomentumDown       MomentumLevel = -1
	MomentumNeutral    MomentumLevel = 0
	MomentumUp         MomentumLevel = 1
	MomentumStrongUp   MomentumLevel = 2
)

type MomentumSignal struct {
	Level     MomentumLevel
	Value     float64 // 原始动能值
	Ret       float64 // 窗口价格收益率
	VolFactor float64 // 成交量放大倍数
}
