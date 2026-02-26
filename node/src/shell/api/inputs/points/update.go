package inputs_points

import "kasper/src/shell/utils/origin"

type UpdateInput struct {
	PointId  string `json:"pointId" validate:"required"`
	IsPublic *bool  `json:"isPublic"`
	PersHist *bool  `json:"persHist"`
	Metadata any    `json:"metadata"`
}

func (d UpdateInput) GetData() any {
	return "dummy"
}

func (d UpdateInput) GetPointId() string {
	return d.PointId
}

func (d UpdateInput) Origin() string {
	return origin.FindOrigin(d.PointId)
}
