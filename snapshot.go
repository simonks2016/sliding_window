package sliding_window

import (
	"fmt"
	"time"
)

type Snapshot struct {
	HighestPrice               float64 `json:"highest_price"`
	LowestPrice                float64 `json:"lowest_price"`
	VolumeWeightedAveragePrice float64 `json:"volume_weighted_average_price"`
	LatestPrice                float64 `json:"latest_price"`
	TotalVolume                float64 `json:"total_volume"`
	BuyVolume                  float64 `json:"buy_volume"`
	SellVolume                 float64 `json:"sell_volume"`
	DeltaVolume                float64 `json:"delta_volume"`
	Momentum                   float64 `json:"momentum"`
	Strength                   float64 `json:"strength"`
	StrengthNorm               float64 `json:"strength_norm"`
	EquPrice                   float64 `json:"equ_price"`
	UpperBand                  float64 `json:"upper_band"`
	LowerBand                  float64 `json:"lower_band"`
	BandWidth                  float64 `json:"band_width"`
	Price                      float64 `json:"price"`
	Distance                   float64 `json:"distance"`
	NormDist                   float64 `json:"norm_dist"`
	NTrades                    int64   `json:"n_trades"`
	WindowMs                   int64   `json:"window_ms"`
	Ts                         int64   `json:"ts"`
	DurationMs                 int64   `json:"duration_ms"`
	Volatility                 float64 `json:"volatility"`
	Imbalance                  float64 `json:"imbalance"`
}

func (w *SlidingWindow) Snapshot() *Snapshot {
	highestPrice := w.HighestPrice.Load()
	lowestPrice := w.LowestPrice.Load()
	latestPrice := w.LatestPrice.Load()
	nTrades := w.nTrades.Load()

	// 这些如果内部会扫窗口/加锁：尽量只调用一次
	n := w.size
	prices, p1 := w.getPricesBuf(n)
	defer w.putPricesBuf(p1)

	stat, ok := w.collectStats(prices)
	if !ok {
		fmt.Println("snapshot not found in sliding window")
		return nil
	}

	vwap, _ := w.vwap(stat)
	//momentum, _ := w.Momentum()
	//bs, _ := w.BreakoutStrength()
	//ez, _ := w.EquilibriumZone(0.4, 0.5)

	// ===== 新增三项 =====
	deltaVol := w.DeltaVolume()
	imb := w.Imbalance()

	rv, okRv := w.RealizedVol()
	if !okRv {
		rv = 0
	}

	totalVolume := w.sumVolume.Float(w.volumeScale)

	return &Snapshot{
		HighestPrice:               QtyLoz(highestPrice).Float(w.priceScale),
		LowestPrice:                QtyLoz(lowestPrice).Float(w.priceScale),
		VolumeWeightedAveragePrice: vwap,
		LatestPrice:                QtyLoz(latestPrice).Float(w.priceScale),
		TotalVolume:                totalVolume,
		BuyVolume:                  float64(w.buyVol.Load()) / float64(w.volumeScale),
		SellVolume:                 float64(w.sellVol.Load()) / float64(w.volumeScale),
		DeltaVolume:                deltaVol,
		Imbalance:                  imb,
		Volatility:                 rv,
		Momentum:                   0.0,
		//Strength:                   bs.Strength,
		//StrengthNorm:               bs.StrengthNorm,
		//EquPrice:                   ez.EquPrice,
		//UpperBand:                  ez.UpperBand,
		//LowerBand:                  ez.LowerBand,
		//BandWidth:                  ez.BandWidth,
		//Price:                      ez.Price,
		//Distance:                   ez.Distance,
		//NormDist:                   ez.NormDist,
		NTrades:    nTrades,
		Ts:         time.Now().UnixMilli(),
		WindowMs:   w.duration.Milliseconds(),
		DurationMs: w.duration.Milliseconds(),
	}
}
