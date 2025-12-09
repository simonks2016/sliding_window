package sliding_window

type EMA struct {
	Value       float64
	Alpha       float64 // 0~1, 越大越偏向新数据
	Initialized bool
}

// NewEMA alpha 一般取 0.01 ~ 0.2 之间看你要多平滑
func NewEMA(alpha float64) *EMA {
	if alpha <= 0 {
		alpha = 0.01
	}
	if alpha > 1 {
		alpha = 1
	}
	return &EMA{Alpha: alpha}
}

// Update 输入当前观察值，返回更新后的 EMA
func (e *EMA) Update(x float64) float64 {
	if !e.Initialized {
		e.Value = x
		e.Initialized = true
		return e.Value
	}
	e.Value = e.Alpha*x + (1-e.Alpha)*e.Value
	return e.Value
}

func (e *EMA) Get() (float64, bool) {

	if !e.Initialized {
		return 0, false
	}
	return e.Value, true
}
