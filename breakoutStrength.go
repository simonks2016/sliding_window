package sliding_window

type BreakoutStrength struct {
	High         float64
	Low          float64
	Price        float64
	Range        float64
	Pos01        float64 // 通道内位置 [0,1]，超出范围也会被 clamp
	Strength     float64 // 突破强度：上破为正，下破为负，未破为 0
	StrengthNorm float64 // 标准化后的突破幅度（相对 Range）
}

func (w *SlidingWindow) BreakoutStrength() (BreakoutStrength, bool) {

	// collectStats：锁内把 prices[0:n] 填满（float 价格），并统计 sumPV/sumV 等
	stats, ok := w.collectStats()
	if !ok {
		return BreakoutStrength{}, false
	}

	return w.breakoutStrength(stats)
}

func (w *SlidingWindow) breakoutStrength(stats WindowStats) (BreakoutStrength, bool) {

	var empty BreakoutStrength
	// 先快照 size，用它申请 buf（collectStats 内部会加锁）
	n := w.size
	if n < 2 {
		return empty, false
	}

	// prices = stats.Prices，本身就是 float 价格（已按 scale 转换）
	// newest 是窗口最后一个点
	price := stats.Prices[n-1]

	// high/low 用前 n-1 个点（排除 newest）
	high := stats.Prices[0]
	low := stats.Prices[0]
	for i := 1; i < n-1; i++ {
		px := stats.Prices[i]
		if px > high {
			high = px
		}
		if px < low {
			low = px
		}
	}

	rng := high - low
	if rng <= 0 {
		return empty, false
	}

	// Pos01：price 在 [low, high] 中的位置（越靠上越接近 1）
	pos := (price - low) / rng
	if pos < 0 {
		pos = 0
	} else if pos > 1 {
		pos = 1
	}

	// Strength：突破距离（与价格同单位），区间内为 0，区间外为正/负
	var s float64
	if price > high {
		s = price - high
	} else if price < low {
		s = -(low - price)
	} else {
		s = 0
	}

	// 标准化突破：无量纲
	strNorm := s / rng

	return BreakoutStrength{
		High:         high,
		Low:          low,
		Price:        price,
		Range:        rng,
		Pos01:        pos,
		Strength:     s,
		StrengthNorm: strNorm,
	}, true

}
