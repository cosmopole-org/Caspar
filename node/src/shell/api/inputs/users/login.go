package inputs_users

type LoginInput struct {
	Username   string `json:"username" validate:"required"`
	EmailToken string `json:"emailToken" validate:"required"`
	Metadata   any    `json:"metadata"`
}

func (d LoginInput) GetData() any {
	return "dummy"
}

func (d LoginInput) GetPointId() string {
	return ""
}

func (d LoginInput) Origin() string {
	return ""
}
