package inputs_points

import "kasper/src/shell/utils/origin"

type AddMemberInput struct {
	UserId   string           `json:"userId" validate:"required"`
	PointId  string           `json:"pointId" validate:"required"`
	Metadata map[string]any   `json:"metadata" validate:"required"`
	Access   map[string]bool `json:"access" validate:"required"`
}

func (d AddMemberInput) GetData() any {
	return "dummy"
}

func (d AddMemberInput) GetPointId() string {
	return d.PointId
}

func (d AddMemberInput) Origin() string {
	return origin.FindOrigin(d.PointId)
}
