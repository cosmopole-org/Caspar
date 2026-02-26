package inputs_pc

type RunPcInput struct{}

func (d RunPcInput) GetData() any {
	return "dummy"
}

func (d RunPcInput) GetPointId() string {
	return ""
}

func (d RunPcInput) Origin() string {
	return ""
}
