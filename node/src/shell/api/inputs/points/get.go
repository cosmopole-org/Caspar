package inputs_points

import "kasper/src/shell/utils/origin"

type GetInput struct {
	PointId     string `json:"pointId" validate:"required"`
	IncludeMeta bool   `json:"includeMeta"`
}

func (d GetInput) GetData() any {
	return "dummy"
}

func (d GetInput) GetPointId() string {
	return d.PointId
}

func (d GetInput) Origin() string {
	o := origin.FindOrigin(d.PointId)
	if o == "global" {
		return ""
	}
	return o
}
