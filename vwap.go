package sliding_window

// VolumeWeightedAveragePrice 计算VWAP价格（复用窗口快照）
func (w *SlidingWindow) VolumeWeightedAveragePrice() (float64, bool) {

	n := w.size
	prices, p1 := w.getPricesBuf(n)
	defer w.putPricesBuf(p1)

	stats, ok := w.collectStats(prices)
	if !ok {
		return 0, false
	}

	if stats.SumV <= 0 {
		return 0, false
	}

	// 转回真实价格
	return float64(stats.SumPV) / float64(stats.SumV), true
}
