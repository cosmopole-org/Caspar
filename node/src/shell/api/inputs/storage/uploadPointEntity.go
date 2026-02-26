package inputs_storage

import "kasper/src/shell/utils/origin"

type UploadPointEntityInput struct {
	Data     string `json:"data" validate:"required"`
	PointId string `json:"pointId" validate:"required"`
	EntityId string `json:"entityId" validate:"required"`
}

func (d UploadPointEntityInput) GetData() any {
	return "dummy"
}

func (d UploadPointEntityInput) GetPointId() string {
	return d.PointId
}

func (d UploadPointEntityInput) Origin() string {
	return origin.FindOrigin(d.PointId)
}
