package inputs_points

type GetDefaultAccessInput struct{}

func (d GetDefaultAccessInput) GetData() any {
	return "dummy"
}

func (d GetDefaultAccessInput) GetPointId() string {
	return ""
}

func (d GetDefaultAccessInput) Origin() string {
	return ""
}
