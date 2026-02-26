package inputs_points

import "kasper/src/shell/utils/origin"

type ListPointAppsInput struct {
	PointId string `json:"pointId" validate:"required"`
}

func (d ListPointAppsInput) GetData() any {
	return "dummy"
}

func (d ListPointAppsInput) GetPointId() string {
	return d.PointId
}

func (d ListPointAppsInput) Origin() string {
	return origin.FindOrigin(d.PointId)
}
