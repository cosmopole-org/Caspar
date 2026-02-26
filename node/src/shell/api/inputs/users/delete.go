package inputs_users

type DeleteInput struct{}

func (d DeleteInput) GetData() any {
	return "dummy"
}

func (d DeleteInput) GetPointId() string {
	return ""
}

func (d DeleteInput) Origin() string {
	return "global"
}
