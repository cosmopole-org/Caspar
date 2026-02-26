package inputs

type HelloInput struct {
	Name string `json:"name"`
}

func (d HelloInput) GetData() any {
	return "dummy"
}

func (d HelloInput) GetPointId() string {
	return ""
}

func (d HelloInput) Origin() string {
	return ""
}