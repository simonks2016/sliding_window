package sliding_window

import "time"

type WindowPoint struct {
	Ts     time.Time // 交易所时间戳
	Price  float64   // 成交价或中价
	Volume float64   // 这一点对应的成交量（或聚合量）
}
