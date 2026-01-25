package sliding_window

import "math"

// 无锁版计算交易量基准（要求调用方已持有 RLock 或 Lock）
func (w *SlidingWindow) volumeFactor() (float64, bool) {
	// 1) baseline：EMA 基准（单位：真实 volume float）
	baselineVol, ok := w.ema.Get()
	if !ok || baselineVol <= 0 {
		return 0, false
	}

	// 2) 当前窗口检查
	sz := w.size
	if sz <= 0 {
		return 0, false
	}

	// 3) 用整数先算平均 units，减少 Float 转换抖动
	sumUnits := int64(w.sumVolume) // QtyLoz 本质 int64
	if sumUnits <= 0 {
		return 0, false
	}

	avgUnitsPerPoint := float64(sumUnits) / float64(sz)  // 仍是 units
	currAvg := avgUnitsPerPoint / float64(w.volumeScale) // 转成真实 volume（只做一次除法）

	if currAvg <= 0 {
		return 0, false
	}

	vf := currAvg / baselineVol
	if vf <= 0 || math.IsNaN(vf) || math.IsInf(vf, 0) {
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
