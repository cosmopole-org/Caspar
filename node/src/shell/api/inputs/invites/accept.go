package inputs_invites

import "kasper/src/shell/utils/origin"

type AcceptInput struct {
	PointId string `json:"pointId" validate:"required"`
}

func (d AcceptInput) GetData() any {
	return "dummy"
}

func (d AcceptInput) GetPointId() string {
	return ""
}

func (d AcceptInput) Origin() string {
	return origin.FindOrigin(d.PointId)
}
