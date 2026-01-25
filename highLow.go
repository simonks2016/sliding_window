package sliding_window

func (w *SlidingWindow) HighLow() (high, low float64, ok bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	// 返回
	return w.highLowUnlocked()
}

func (w *SlidingWindow) highLowUnlocked() (float64, float64, bool) {

	if w.size == 0 {
		return 0, 0, false
	}

	first := w.atUnlocked(0).Price
	high, low := first, first

	for i := 1; i < w.size; i++ {
		p := w.atUnlocked(i).Price
		if p > high {
			high = p
		}
		if p < low {
			low = p
		}
	}

	return high.Float(w.priceScale), low.Float(w.priceScale), true
}
