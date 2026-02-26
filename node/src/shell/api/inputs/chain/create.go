package inputs_machiner

type CreateInput struct {
	IsTemp *bool `json:"isTemp" validate:"required"`
}

func (d CreateInput) GetData() any {
	return "dummy"
}

func (d CreateInput) GetPointId() string {
	return ""
}

func (d CreateInput) Origin() string {
	return "global"
}
