package inputs_machiner

type ListInput struct {
	Offset int64 `json:"offset"`
	Count  int64 `json:"count"`
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
