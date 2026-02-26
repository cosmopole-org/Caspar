package inputs_storage

type UploadUserEntityInput struct {
	Data      string `json:"data" validate:"required"`
	EntityId  string `json:"entityId" validate:"required"`
	MachineId string `json:"machineId"`
}

func (d UploadUserEntityInput) GetData() any {
	return "dummy"
}

func (d UploadUserEntityInput) GetPointId() string {
	return ""
}

func (d UploadUserEntityInput) Origin() string {
	return "global"
}
