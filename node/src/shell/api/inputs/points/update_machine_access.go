package inputs_points

import "kasper/src/shell/utils/origin"

type UpdateMachineAccessInput struct {
	MachineId  string          `json:"machineId" validate:"required"`
	PointId    string          `json:"pointId" validate:"required"`
	Access     map[string]bool `json:"access" validate:"required"`
}

func (d UpdateMachineAccessInput) GetData() any {
	return "dummy"
}

func (d UpdateMachineAccessInput) GetPointId() string {
	return d.PointId
}

func (d UpdateMachineAccessInput) Origin() string {
	return origin.FindOrigin(d.PointId)
}
