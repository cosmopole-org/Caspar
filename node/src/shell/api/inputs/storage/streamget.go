package inputs_storage

type StreamGetInput struct {
	MachineId string `json:"machineId" validate:"required"`
	PointId   string `json:"pointId" validate:"required"`
	Metadata  string `json:"metadata" validate:"required"`
}

func (d StreamGetInput) GetData() any {
	return "dummy"
}

func (d StreamGetInput) GetPointId() string {
	return d.PointId
}

func (d StreamGetInput) Origin() string {
	return ""
}
