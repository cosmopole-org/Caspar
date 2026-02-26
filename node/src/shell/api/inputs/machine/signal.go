package inputs_machiner

type SignalInput struct {
	Data      string `json:"data" validate:"required"`
	MachineId string `json:"machineId" validate:"required"`
	VmTag     string `json:"vmTag" validate:"required"`
}

func (d SignalInput) GetData() any {
	return "dummy"
}

func (d SignalInput) GetPointId() string {
	return ""
}

func (d SignalInput) Origin() string {
	return ""
}
