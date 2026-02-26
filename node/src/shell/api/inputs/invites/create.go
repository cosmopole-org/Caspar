package inputs_invites

import "kasper/src/shell/utils/origin"

type CreateInput struct {
	PointId string `json:"pointId" validate:"required"`
	UserId  string `json:"userId" validate:"required"`
}

func (d CreateInput) GetData() any {
	return "dummy"
}

func (d CreateInput) GetPointId() string {
	return d.PointId
}

func (d CreateInput) Origin() string {
	return origin.FindOrigin(d.PointId)
}
