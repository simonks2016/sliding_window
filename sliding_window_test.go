package sliding_window

import (
	"fmt"
	"testing"
	"time"
)

func BenchmarkEquilibriumZone(b *testing.B) {
	w := NewSlidingWindow(time.Second, 4096, 0.2)

	// 先灌满窗口，避免测试阶段的扩容/初始化噪声
	for i := 0; i < 4096; i++ {
		w.AddWindowPoint(
			SideBuy,
			990+float64(i%10)*0.01,
			1,
			time.Now().Add(time.Duration(i)*time.Millisecond),
		)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		//_, _ = w.MedianPrice()
		_, ok := w.Momentum()
		if !ok {
			fmt.Println("not ok")
		}
	}
}
