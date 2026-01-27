package sliding_window

import "sort"

// MedianPrice  对外带锁，锁内只复制，锁外排序计算
func (w *SlidingWindow) MedianPrice() (float64, bool) {

	stats, ok := w.collectStats() // collectStats 内部把 prices 填满
	if !ok {
		return 0, false
	}

	return w.medianPrice(stats)
}

func (w *SlidingWindow) medianPrice(stats WindowStats) (float64, bool) {

	// 直接对 prices 排序（它就是 stats.Prices）
	sort.Float64s(stats.Prices)

	n := len(stats.Prices)
	var med float64
	if n%2 == 1 {
		med = stats.Prices[n/2]
	} else {
		med = (stats.Prices[n/2-1] + stats.Prices[n/2]) / 2.0
	}
	return med, true
}
