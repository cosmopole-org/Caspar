package inputs_machiner

type RunMachineInput struct {
	MachineId string `json:"machineId" validate:"required"`
}

func (d RunMachineInput) GetData() any {
	return "dummy"
}

func (d RunMachineInput) GetPointId() string {
	return ""
}

func (d RunMachineInput) Origin() string {
	return "global"
}
