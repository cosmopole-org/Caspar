package inputs_machiner

type ListAppMachsInput struct {
	AppId string `json:"appId" validate:"required"`
}

func (d ListAppMachsInput) GetData() any {
	return "dummy"
}

func (d ListAppMachsInput) GetPointId() string {
	return ""
}

func (d ListAppMachsInput) Origin() string {
	return ""
}
