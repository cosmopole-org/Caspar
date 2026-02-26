package inputs_invites

import "kasper/src/shell/utils/origin"

type DeclineInput struct {
	PointId string `json:"pointId" validate:"required"`
}

func (d DeclineInput) GetData() any {
	return "dummy"
}

func (d DeclineInput) GetPointId() string {
	return ""
}

func (d DeclineInput) Origin() string {
	return origin.FindOrigin(d.PointId)
}
