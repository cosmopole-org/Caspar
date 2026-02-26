package inputs_machiner

type UpdateAppInput struct {
	AppId    string `json:"appId" validate:"required"`
	Metadata any    `json:"metadata" validate:"required"`
}

func (d UpdateAppInput) GetData() any {
	return "dummy"
}

func (d UpdateAppInput) GetPointId() string {
	return ""
}

func (d UpdateAppInput) Origin() string {
	return "global"
}
