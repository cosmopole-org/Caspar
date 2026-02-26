package inputs_points

import "kasper/src/shell/utils/origin"

type JoinInput struct {
	PointId string `json:"pointId" validate:"required"`
}

func (d JoinInput) GetData() any {
	return "dummy"
}

func (d JoinInput) GetPointId() string {
	return d.PointId
}

func (d JoinInput) Origin() string {
	return origin.FindOrigin(d.PointId)
}
