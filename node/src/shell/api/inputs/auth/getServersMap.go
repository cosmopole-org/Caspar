package inputs_auth

type GetServersMapInput struct{}

func (d GetServersMapInput) GetData() any {
	return "dummy"
}

func (d GetServersMapInput) GetPointId() string {
	return ""
}

func (d GetServersMapInput) Origin() string {
	return ""
}