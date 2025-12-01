package sliding_window

type MomentumLevel int

const (
	MomentumStrongDown MomentumLevel = -2
	MomentumDown       MomentumLevel = -1
	MomentumNeutral    MomentumLevel = 0
	MomentumUp         MomentumLevel = 1
	MomentumStrongUp   MomentumLevel = 2
)

type MomentumSignal struct {
	Level     MomentumLevel
	Value     float64 // 原始动能值
	Ret       float64 // 窗口价格收益率
	VolFactor float64 // 成交量放大倍数
}
