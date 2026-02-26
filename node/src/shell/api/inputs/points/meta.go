package inputs_points

import "kasper/src/shell/utils/origin"

type MetaInput struct {
	PointId string `json:"pointId" validate:"required"`
	Path    string `json:"path" validate:"required"`
}

func (d MetaInput) GetData() any {
	return "dummy"
}

func (d MetaInput) GetPointId() string {
	return d.PointId
}

func (d MetaInput) Origin() string {
	o := origin.FindOrigin(d.PointId)
	if o == "global" {
		return ""
	}
	return o
}
