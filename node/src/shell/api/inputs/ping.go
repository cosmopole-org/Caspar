package inputs

type PingInput struct {
}

func (d PingInput) GetData() any {
	return "dummy"
}

func (d PingInput) GetPointId() string {
	return ""
}

func (d PingInput) Origin() string {
	return ""
}