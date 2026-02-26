package inputs_points

type ListInput struct {
	Offset int64  `json:"offset"`
	Count  int64  `json:"count"`
	Query  string `json:"query"`
}

func (d ListInput) GetData() any {
	return "dummy"
}

func (d ListInput) GetPointId() string {
	return ""
}

func (d ListInput) Origin() string {
	return ""
}
