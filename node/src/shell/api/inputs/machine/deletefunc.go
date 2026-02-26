package inputs_machiner

type DeleteMachineInput struct {
	MachineId string `json:"machineId" validate:"required"`
}

func (d DeleteMachineInput) GetData() any {
	return "dummy"
}

func (d DeleteMachineInput) GetPointId() string {
	return ""
}

func (d DeleteMachineInput) Origin() string {
	return "global"
}
