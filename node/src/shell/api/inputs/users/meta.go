package inputs_users

type MetaInput struct {
	UserId string `json:"userId" validate:"required"`
	Path   string `json:"path" validate:"required"`
}

func (d MetaInput) GetData() any {
	return "dummy"
}

func (d MetaInput) GetPointId() string {
	return ""
}

func (d MetaInput) Origin() string {
	return "global"
}
