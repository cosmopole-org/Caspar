package inputs_points

import "kasper/src/shell/utils/origin"

type MachineMeta struct {
	MachineId  string          `json:"machineId" validate:"required"`
	Identifier string          `json:"identifier" validate:"required"`
	Access     map[string]bool `json:"access" validate:"required"`
	Metadata   map[string]any  `json:"metadata"`
}

type AddAppInput struct {
	AppId        string        `json:"appId" validate:"required"`
	PointId      string        `json:"pointId" validate:"required"`
	MachinesMeta []MachineMeta `json:"machinesMeta" validate:"required"`
}

func (d AddAppInput) GetData() any {
	return "dummy"
}

func (d AddAppInput) GetPointId() string {
	return d.PointId
}

func (d AddAppInput) Origin() string {
	return origin.FindOrigin(d.PointId)
}
