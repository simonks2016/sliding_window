package sliding_window

import "math"

func (w *SlidingWindow) structuralReturn() (float64, bool) {

	if w.size <= 0 || len(w.buf) <= 0 {
		return 0, false
	}

	old := w.atUnlocked(0)
	newest := w.lastUnlocked()

	// 价格收益率
	if old.Price == 0 {
		return 0, false
	}

	return (newest.Price.Float(w.priceScale) - old.Price.Float(w.priceScale)) / old.Price.Float(w.priceScale), true
}

// DeltaVolume: buy - sell （单位：成交量，已经除以 volumeScale 后的真实值）
func (w *SlidingWindow) DeltaVolume() float64 {
	bv := float64(w.buyVol.Load()) / float64(w.volumeScale)
	sv := float64(w.sellVol.Load()) / float64(w.volumeScale)
	return bv - sv
}

// Imbalance: (buy - sell) / (buy + sell)，范围 [-1, 1]
func (w *SlidingWindow) Imbalance() float64 {
	bv := float64(w.buyVol.Load()) / float64(w.volumeScale)
	sv := float64(w.sellVol.Load()) / float64(w.volumeScale)
	den := bv + sv
	if den <= 0 {
		return 0
	}
	return (bv - sv) / den
}

// RealizedVol: sqrt(sum(log(p_i/p_{i-1})^2))，窗口内 realized vol（不年化）
func (w *SlidingWindow) RealizedVol() (float64, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.size < 2 {
		return 0, false
	}

	// 用 float 价格做 log return（缩放用 Float 一次就够）
	prev := w.atUnlocked(0).Price.Float(w.priceScale)
	if prev <= 0 {
		return 0, false
	}

	var sumsq float64
	for i := 1; i < w.size; i++ {
		cur := w.atUnlocked(i).Price.Float(w.priceScale)
		if cur <= 0 {
			prev = cur
			continue
		}
		r := math.Log(cur / prev)
		sumsq += r * r
		prev = cur
	}

	return math.Sqrt(sumsq), true
}
