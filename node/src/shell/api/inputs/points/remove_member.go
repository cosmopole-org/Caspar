package inputs_points

import "kasper/src/shell/utils/origin"

type RemoveMemberInput struct {
	UserId  string `json:"userId" validate:"required"`
	PointId string `json:"pointId" validate:"required"`
}

func (d RemoveMemberInput) GetData() any {
	return "dummy"
}

func (d RemoveMemberInput) GetPointId() string {
	return d.PointId
}

func (d RemoveMemberInput) Origin() string {
	return origin.FindOrigin(d.PointId)
}
