package sliding_window

import "time"

type WindowPoint struct {
	Ts     time.Time `json:"ts"`
	Price  QtyLoz    `json:"price"`
	Volume QtyLoz    `json:"volume"`
	Side   Side      `json:"side"`
}

type Side uint8

const (
	SideUnknown Side = iota
	SideBuy
	SideSell
)
