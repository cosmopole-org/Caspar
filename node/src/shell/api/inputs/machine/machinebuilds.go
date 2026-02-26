package inputs_machiner

type MachineBuildsInput struct {
	MachineId string `json:"machineId" validate:"required"`
	Offset    int    `json:"offset" validate:"required"`
	Count     int    `json:"count" validate:"required"`
}

func (d MachineBuildsInput) GetData() any {
	return "dummy"
}

func (d MachineBuildsInput) GetPointId() string {
	return ""
}

func (d MachineBuildsInput) Origin() string {
	return "global"
}
