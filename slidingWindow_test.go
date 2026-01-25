package sliding_window

import (
	"math/rand"
	"runtime"
	"testing"
	"time"
)

func TestSlidingWindow_StreamMomentumPerf(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	const (
		windowSize = 20000
		runSeconds = 5
		priceBase  = int64(100_000)
		jitter     = int64(30)
		volMin     = int64(1)
		volMax     = int64(20)

		snapshotEvery = 10 * time.Millisecond
	)

	w := NewSlidingWindow(
		time.Minute,
		windowSize,
		0.03,
	)

	// ==== 流式行情生成器 ====
	type tradeGen struct {
		ts    time.Time
		price int64
	}

	gen := &tradeGen{
		ts:    time.Now(),
		price: priceBase,
	}

	nextTrade := func() WindowPoint {
		gen.price += rand.Int63n(jitter*2+1) - jitter
		if gen.price <= 0 {
			gen.price = priceBase
		}
		vol := volMin + rand.Int63n(volMax-volMin+1)

		isBuy := true
		r := rand.Int()
		if r%2 == 0 {
			isBuy = false
		}

		pt := WindowPoint{
			Ts:     gen.ts,
			Price:  NewQtyLoz(float64(gen.price), NewQtyScaleFromDecimals(2)),
			Volume: NewQtyLoz(float64(vol), NewQtyScaleFromDecimals(8)),
			Side: func() Side {
				if isBuy {
					return SideBuy
				}
				return SideSell
			}(),
			// Side: 你如果已经加了 Side，可以在这里随机赋值
		}
		gen.ts = gen.ts.Add(time.Millisecond)
		return pt
	}

	// ==== 预热窗口 ====
	warm := make([]WindowPoint, windowSize)
	for i := 0; i < windowSize; i++ {
		warm[i] = nextTrade()
	}
	w.Add(warm...)

	// ==== 定时 snapshot 打印 ====
	ticker := time.NewTicker(snapshotEvery)
	defer ticker.Stop()

	timeout := time.NewTimer(time.Duration(runSeconds) * time.Second)
	defer timeout.Stop()

	// ==== 性能统计（按 trade） ====
	var calls int64
	var totalLatency time.Duration

	// 如果你想把 Snapshot() 的调用成本也统计进去，可以加这两个
	var snapCalls int64
	var snapTotal time.Duration

	for {
		select {
		case <-timeout.C:
			goto DONE

		case <-ticker.C:
			st := time.Now()
			s := w.Snapshot()
			snapTotal += time.Since(st)
			snapCalls++

			// ⚠️ 每 10ms Log 一次会非常吵，也会影响性能数据（testing.Log 本身挺重）
			// 但你明确要“打印信息”，这里就直接打印；如果你想更真实的吞吐，建议改成每 100ms 打一次，或只统计不打。
			t.Logf(
				"[Snap] ts=%d price=%.2f vwap=%.2f mom=%.6f eq=%.2f bw=%.4f nd=%.4f dv=%.4f imb=%.4f rv=%.6f trades=%d",
				s.Ts,
				s.LatestPrice,
				s.VolumeWeightedAveragePrice,
				s.Momentum,
				s.EquPrice,
				s.BandWidth,
				s.NormDist,
				s.DeltaVolume,
				s.Imbalance,
				s.Volatility,
				s.NTrades,
			)

		default:
			// 一直喂 trade（尽量快）
			pt := nextTrade()

			t0 := time.Now()
			w.Add(pt)
			_, ok := w.Momentum() // 你要的是边 add 边算动能
			cost := time.Since(t0)

			if ok {
				calls++
				totalLatency += cost
			}
		}
	}

DONE:
	avgLatency := time.Duration(0)
	if calls > 0 {
		avgLatency = totalLatency / time.Duration(calls)
	}

	t.Logf("Trades processed: %d", calls)
	t.Logf("Throughput: %.0f trades/sec", float64(calls)/float64(runSeconds))
	t.Logf("Avg latency per trade: %v", avgLatency)

	if snapCalls > 0 {
		t.Logf("Snapshot calls: %d (%.1f / sec)", snapCalls, float64(snapCalls)/float64(runSeconds))
		t.Logf("Avg Snapshot cost: %v", snapTotal/time.Duration(snapCalls))
	}

	// ==== 内存 ====
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	t.Logf(
		"Memory: Alloc=%d KB Heap=%d KB Sys=%d KB NumGC=%d",
		m.Alloc/1024,
		m.HeapAlloc/1024,
		m.Sys/1024,
		m.NumGC,
	)
}
