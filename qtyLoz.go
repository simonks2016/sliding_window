package sliding_window

import (
	"math"
)

type QtyLoz int64

func NewQtyLoz(n float64, scale QtyScale) QtyLoz {
	// epsilon 抵抗浮点边界误差
	x := n*float64(scale) + 1e-9
	return QtyLoz(math.Round(x))
}

func (q QtyLoz) Float(scale QtyScale) float64 {
	return float64(q) / float64(scale)
}
func (q QtyLoz) Add(b QtyLoz) QtyLoz { return q + b }
func (q QtyLoz) Sub(b QtyLoz) QtyLoz { return q - b }
func (q QtyLoz) IsZero() bool        { return q == 0 }
func (q QtyLoz) Abs() QtyLoz {
	if q < 0 {
		return -q
	}
	return q
}
func (q QtyLoz) Int64() int64 { return int64(q) }

type QtyScale int64

func NewQtyScaleFromDecimals(decimals int) QtyScale {
	decimals = max(decimals, 1)
	decimals = min(decimals, 18)

	var scale int64 = 1
	for i := 0; i < decimals; i++ {
		scale *= 10
	}

	return QtyScale(scale)
}
