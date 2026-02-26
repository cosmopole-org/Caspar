package inputs_points

import "kasper/src/shell/utils/origin"

type UpdateMemberInput struct {
	UserId   string         `json:"memberId" validate:"required"`
	PointId  string         `json:"pointId" validate:"required"`
	Metadata map[string]any `json:"metadata" validate:"required"`
}

func (d UpdateMemberInput) GetData() any {
	return "dummy"
}

func (d UpdateMemberInput) GetPointId() string {
	return d.PointId
}

func (d UpdateMemberInput) Origin() string {
	return origin.FindOrigin(d.PointId)
}
