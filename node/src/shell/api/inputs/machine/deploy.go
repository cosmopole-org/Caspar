package inputs_machiner

type DeployInput struct {
	MachineId string         `json:"machineId" validate:"required"`
	ByteCode  string         `json:"byteCode" validate:"required"`
	Runtime   string         `json:"runtime" validate:"required"`
	Metadata  map[string]any `json:"metadata"`
}

func (d DeployInput) GetData() any {
	return "dummy"
}

func (d DeployInput) GetPointId() string {
	return ""
}

func (d DeployInput) Origin() string {
	return "global"
}
