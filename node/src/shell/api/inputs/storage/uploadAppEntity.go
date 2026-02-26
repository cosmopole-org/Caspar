package inputs_storage

type UploadAppEntityInput struct {
	Data     string `json:"data" validate:"required"`
	AppId    string `json:"AppId" validate:"required"`
	EntityId string `json:"entityId" validate:"required"`
}

func (d UploadAppEntityInput) GetData() any {
	return "dummy"
}

func (d UploadAppEntityInput) GetPointId() string {
	return ""
}

func (d UploadAppEntityInput) Origin() string {
	return "global"
}
