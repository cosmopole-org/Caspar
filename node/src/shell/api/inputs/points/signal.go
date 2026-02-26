package inputs_points

type SignalInput struct {
	Type       string `json:"type" validate:"required"`
	Data       string `json:"data" validate:"required"`
	PointId    string `json:"pointId" validate:"required"`
	UserId     string `json:"userId"`
	Temp       bool   `json:"temp"`
	EditSignal bool   `json:"editSignal"`
}

func (d SignalInput) GetData() any {
	return "dummy"
}

func (d SignalInput) GetPointId() string {
	return d.PointId
}

func (d SignalInput) Origin() string {
	return ""
}
