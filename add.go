package sliding_window

import "time"

// add 无锁批量添加
// add 无锁批量添加（假设外层已经 w.mu.Lock 住）
func (w *SlidingWindow) add(pts ...WindowPoint) {
	if len(pts) == 0 {
		return
	}

	lastTs := pts[len(pts)-1].Ts
	threshold := lastTs.Add(-w.duration)

	for i := range pts {
		pt := pts[i]
		if !pt.Ts.After(threshold) {
			continue
		}

		if w.size == 0 {
			w.buf[0] = pt
			w.start = 0
			w.size = 1

			// 新增统计
			w.applyAddPointUnlocked(pt)

		} else if w.size < len(w.buf) {
			idx := (w.start + w.size) % len(w.buf)
			w.buf[idx] = pt
			w.size++

			w.applyAddPointUnlocked(pt)

		} else {
			// 覆盖头部
			idx := w.start
			old := w.buf[idx]

			// 先减旧点统计
			w.applyRemovePointUnlocked(old)

			// 覆盖
			w.buf[idx] = pt
			w.start = (w.start + 1) % len(w.buf)

			// 再加新点统计
			w.applyAddPointUnlocked(pt)
		}
	}

	// trim：把“窗口内残留过期点”清掉（你原本就有）
	w.trimExpiredUnlocked(threshold) // ⚠️ 这里也要同步做 applyRemove（见下）

	// high/low 若 dirty，补一次
	w.recomputeHighLowIfDirtyUnlocked()

	// 你原本的缓存刷新
	w.refreshVolumeCachesUnlocked()
}

// trimExpiredUnlocked：移除所有 Ts <= threshold 的点（保持窗口为 (threshold, +inf]）
func (w *SlidingWindow) trimExpiredUnlocked(threshold time.Time) {
	for w.size > 0 {
		head := w.buf[w.start]
		if head.Ts.After(threshold) {
			break
		}
		// 移除 head
		w.applyRemovePointUnlocked(head)

		w.start = (w.start + 1) % len(w.buf)
		w.size--
	}

	if w.size == 0 {
		// 清空 latest/high/low 的合理处理（可选）
		w.LatestPrice.Store(0)
		w.hiLoDirty = false
		w.HighestPrice.Store(0)
		w.LowestPrice.Store(0)
	} else {
		// latest 也可在 trim 后重新设（可选）
		lastIdx := (w.start + w.size - 1) % len(w.buf)
		w.LatestPrice.Store(w.buf[lastIdx].Price.Int64())
	}
}

func (w *SlidingWindow) refreshVolumeCachesUnlocked() {
	if w.size <= 0 {
		w.avgVolPerPoint.Store(0)
		w.volPerSecond.Store(0)
		return
	}

	// === 平均每点成交量（ticks） ===
	// sumVolume 是 QtyLoz（内部是 ticks）
	avgTicks := int64(w.sumVolume) / int64(w.size)
	w.avgVolPerPoint.Store(avgTicks)

	// === 每秒成交量（ticks / second） ===
	if w.size < 2 {
		w.volPerSecond.Store(0)
		return
	}

	oldest := w.atUnlocked(0)
	newest := w.lastUnlocked()

	sec := newest.Ts.Sub(oldest.Ts).Seconds()
	if sec <= 0 {
		w.volPerSecond.Store(0)
		return
	}

	// ticks / second
	vps := float64(w.sumVolume) / sec
	w.volPerSecond.Store(int64(vps))
}

// Add 添加一个点并自动清理超出时间窗口的旧点（写锁）
func (w *SlidingWindow) Add(p ...WindowPoint) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.add(p...)
	return
}

// AddWindowPoint 添加一个点并自动清理超出时间窗口的旧点（写锁）
func (w *SlidingWindow) AddWindowPoint(side Side, price, size float64, ts time.Time) {

	w.mu.Lock()
	defer w.mu.Unlock()

	w.add(WindowPoint{
		Ts:     ts,
		Price:  NewQtyLoz(price, w.priceScale),
		Volume: NewQtyLoz(size, w.volumeScale),
		Side:   side,
	})
	return
}

func (w *SlidingWindow) recomputeHighLowIfDirtyUnlocked() {
	if !w.hiLoDirty {
		return
	}
	if w.size == 0 {
		w.HighestPrice.Store(0)
		w.LowestPrice.Store(0)
		w.hiLoDirty = false
		return
	}

	first := w.buf[w.start]
	hi := first.Price.Int64()
	lo := hi

	for i := 1; i < w.size; i++ {
		idx := (w.start + i) % len(w.buf)
		px := w.buf[idx].Price.Int64()
		if px > hi {
			hi = px
		}
		if px < lo {
			lo = px
		}
	}

	w.HighestPrice.Store(hi)
	w.LowestPrice.Store(lo)
	w.hiLoDirty = false
}

func (w *SlidingWindow) applyAddPointUnlocked(pt WindowPoint) {
	// === 原有 sumVolume / EMA ===
	w.sumVolume += pt.Volume
	if int64(pt.Volume) > 0 {
		w.ema.Update(float64(pt.Volume) / float64(w.volumeScale))
	}

	// === 新增：ticks 统计 ===
	px := pt.Price.Int64()
	v := pt.Volume.Int64()
	if v < 0 {
		v = 0
	} // 防御

	// trades 计数（你如果想 Unknown side 也算一次 trade，就放这里）
	w.nTrades.Add(1)

	// SumV / SumPV（注意：px*v 可能溢出，见后面说明）
	w.SumV.Add(v)
	w.SumPV.Add(px * v)

	// buy/sell vol
	switch pt.Side {
	case SideBuy:
		w.buyVol.Add(v)
	case SideSell:
		w.sellVol.Add(v)
	default:
		return
	}

	// latest
	w.LatestPrice.Store(px)

	// high / low：增量更新（只有变大/变小才写）
	for {
		cur := w.HighestPrice.Load()
		if cur == 0 || px > cur {
			if w.HighestPrice.CompareAndSwap(cur, px) {
				break
			}
			continue
		}
		break
	}

	for {
		cur := w.LowestPrice.Load()
		if cur == 0 || px < cur {
			if w.LowestPrice.CompareAndSwap(cur, px) {
				break
			}
			continue
		}
		break
	}
}

func (w *SlidingWindow) applyRemovePointUnlocked(pt WindowPoint) {
	w.sumVolume -= pt.Volume

	px := pt.Price.Int64()
	v := pt.Volume.Int64()
	if v < 0 {
		v = 0
	}

	w.nTrades.Add(-1)
	w.SumV.Add(-v)
	w.SumPV.Add(-(px * v))

	switch pt.Side {
	case SideBuy:
		w.buyVol.Add(-v)
	case SideSell:
		w.sellVol.Add(-v)
	default:
		return
	}

	// 如果删掉的点“可能是最高/最低”，标记 dirty，稍后必要时重算
	if px == w.HighestPrice.Load() || px == w.LowestPrice.Load() {
		w.hiLoDirty = true
	}
}
