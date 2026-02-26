package inputs_points

import "kasper/src/shell/utils/origin"

type UpdateMachineInput struct {
	PointId     string      `json:"pointId" validate:"required"`
	AppId       string      `json:"appId" validate:"required"`
	MachineMeta MachineMeta `json:"machineMeta" validate:"required"`
}

func (d UpdateMachineInput) GetData() any {
	return "dummy"
}

func (d UpdateMachineInput) GetPointId() string {
	return d.PointId
}

func (d UpdateMachineInput) Origin() string {
	return origin.FindOrigin(d.PointId)
}
