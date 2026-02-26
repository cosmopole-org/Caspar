package inputs_users

type FindInput struct {
	Username string `json:"username" validate:"required"`
}

func (d FindInput) GetData() any {
	return "dummy"
}

func (d FindInput) GetPointId() string {
	return ""
}

func (d FindInput) Origin() string {
	return ""
}
