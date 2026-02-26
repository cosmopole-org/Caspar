package inputs_points

import "kasper/src/shell/utils/origin"

type RemoveMachineInput struct {
	AppId      string `json:"appId" validate:"required"`
	PointId    string `json:"pointId" validate:"required"`
	MachineId  string `json:"machineId" validate:"required"`
	Identifier string `json:"identifier" validate:"required"`
}

func (d RemoveMachineInput) GetData() any {
	return "dummy"
}

func (d RemoveMachineInput) GetPointId() string {
	return d.PointId
}

func (d RemoveMachineInput) Origin() string {
	return origin.FindOrigin(d.PointId)
}
