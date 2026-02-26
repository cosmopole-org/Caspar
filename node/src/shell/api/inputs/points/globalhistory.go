package inputs_points

type GlobalHistoryInput struct {
	Count int      `json:"count" validate:"required"`
	Ids   []string `json:"Ids" validate:"required"`
}

func (d GlobalHistoryInput) GetData() any {
	return "dummy"
}

func (d GlobalHistoryInput) GetPointId() string {
	return ""
}

func (d GlobalHistoryInput) Origin() string {
	return ""
}
