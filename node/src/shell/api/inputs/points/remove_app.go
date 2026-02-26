package inputs_points

import "kasper/src/shell/utils/origin"

type RemoveAppInput struct {
	AppId   string `json:"appId" validate:"required"`
	PointId string `json:"pointId" validate:"required"`
}

func (d RemoveAppInput) GetData() any {
	return "dummy"
}

func (d RemoveAppInput) GetPointId() string {
	return d.PointId
}

func (d RemoveAppInput) Origin() string {
	return origin.FindOrigin(d.PointId)
}
