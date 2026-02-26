package inputs_points

import "kasper/src/shell/utils/origin"

type DeleteInput struct {
	PointId string `json:"pointId" validate:"required"`
}

func (d DeleteInput) GetData() any {
	return "dummy"
}

func (d DeleteInput) GetPointId() string {
	return d.PointId
}

func (d DeleteInput) Origin() string {
	return origin.FindOrigin(d.PointId)
}
