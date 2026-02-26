package inputs_users

type UpdateInput struct {
	Metadata any `json:"metadata"`
}

func (d UpdateInput) GetData() any {
	return "dummy"
}

func (d UpdateInput) GetPointId() string {
	return ""
}

func (d UpdateInput) Origin() string {
	return "global"
}
