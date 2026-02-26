package inputs

type EmptyInput struct {
}

func (d EmptyInput) GetData() any {
	return "dummy"
}

func (d EmptyInput) GetPointId() string {
	return ""
}

func (d EmptyInput) Origin() string {
	return ""
}