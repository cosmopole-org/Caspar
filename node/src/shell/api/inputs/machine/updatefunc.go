package inputs_machiner

type UpdateMachineInput struct {
	MachineId string         `json:"machineId" validate:"required"`
	Path      string         `json:"path" validate:"required"`
	Metadata  map[string]any `json:"metadata"`
}

func (d UpdateMachineInput) GetData() any {
	return "dummy"
}

func (d UpdateMachineInput) GetPointId() string {
	return ""
}

func (d UpdateMachineInput) Origin() string {
	return "global"
}
