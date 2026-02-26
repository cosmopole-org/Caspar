package inputs_machiner

type DeleteAppInput struct {
	AppId string `json:"appId" validate:"required"`
}

func (d DeleteAppInput) GetData() any {
	return "dummy"
}

func (d DeleteAppInput) GetPointId() string {
	return ""
}

func (d DeleteAppInput) Origin() string {
	return "global"
}
