package inputs_machiner

type RegisterNodeInput struct {
	Orig string `json:"orig" validate:"required"`
}

func (d RegisterNodeInput) GetData() any {
	return "dummy"
}

func (d RegisterNodeInput) GetPointId() string {
	return ""
}

func (d RegisterNodeInput) Origin() string {
	return "global"
}
