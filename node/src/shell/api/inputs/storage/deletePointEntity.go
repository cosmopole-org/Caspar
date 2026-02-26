package inputs_storage

import "kasper/src/shell/utils/origin"

type DeletePointEntityInput struct {
	PointId string `json:"pointId" validate:"required"`
	EntityId string `json:"entityId" validate:"required"`
}

func (d DeletePointEntityInput) GetData() any {
	return "dummy"
}

func (d DeletePointEntityInput) GetPointId() string {
	return d.PointId
}

func (d DeletePointEntityInput) Origin() string {
	return origin.FindOrigin(d.PointId)
}
