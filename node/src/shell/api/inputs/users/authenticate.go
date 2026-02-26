package inputs_users

type AuthenticateInput struct{}

func (d AuthenticateInput) GetData() any {
	return "dummy"
}

func (d AuthenticateInput) GetPointId() string {
	return ""
}

func (d AuthenticateInput) Origin() string {
	return ""
}
