package inputs_storage

type DeleteUserEntityInput struct {
	EntityId string `json:"entityId" validate:"required"`
	MachineId string `json:"machineId"`
}

func (d DeleteUserEntityInput) GetData() any {
	return "dummy"
}

func (d DeleteUserEntityInput) GetPointId() string {
	return ""
}

func (d DeleteUserEntityInput) Origin() string {
	return "global"
}
