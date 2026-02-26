package inputs_points

import "kasper/src/shell/utils/origin"

type UpdateMemberAccessInput struct {
	UserId  string          `json:"memberId" validate:"required"`
	PointId string          `json:"pointId" validate:"required"`
	Access  map[string]bool `json:"access" validate:"required"`
}

func (d UpdateMemberAccessInput) GetData() any {
	return "dummy"
}

func (d UpdateMemberAccessInput) GetPointId() string {
	return d.PointId
}

func (d UpdateMemberAccessInput) Origin() string {
	return origin.FindOrigin(d.PointId)
}
