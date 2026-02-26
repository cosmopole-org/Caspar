package inputs_points

type ReadInput struct {
	Offset int64  `json:"offset"`
	Count  int64  `json:"count"`
	Tag    string `json:"tag"`
	Orig   string `json:"orig"`
}

func (d ReadInput) GetData() any {
	return "dummy"
}

func (d ReadInput) GetPointId() string {
	return ""
}

func (d ReadInput) Origin() string {
	return d.Orig
}
