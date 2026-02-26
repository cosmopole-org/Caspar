package inputs_invites

import "kasper/src/shell/utils/origin"

type CancelInput struct {
	PointId string `json:"pointId" validate:"required"`
	UserId  string `json:"userId" validate:"required"`
}

func (d CancelInput) GetData() any {
	return "dummy"
}

func (d CancelInput) GetPointId() string {
	return d.PointId
}

func (d CancelInput) Origin() string {
	return origin.FindOrigin(d.PointId)
}
