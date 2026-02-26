package inputs_points

import "kasper/src/shell/utils/origin"

type ReadMemberInput struct {
	PointId string `json:"pointId" validate:"required"`
}

func (d ReadMemberInput) GetData() any {
	return "dummy"
}

func (d ReadMemberInput) GetPointId() string {
	return d.PointId
}

func (d ReadMemberInput) Origin() string {
	return origin.FindOrigin(d.PointId)
}
