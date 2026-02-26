package inputs_auth

type GetServerKeyInput struct{}

func (d GetServerKeyInput) GetData() any {
	return "dummy"
}

func (d GetServerKeyInput) GetPointId() string {
	return ""
}

func (d GetServerKeyInput) Origin() string {
	return ""
}
