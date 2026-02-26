package inputs_storage

import (
	"kasper/src/shell/utils/origin"
)

type UploadDataInput struct {
	Data    string `json:"data" validate:"required"`
	PointId string `json:"pointId" validate:"required"`
	FileId  string `json:"fileId"`
}

func (d UploadDataInput) GetData() any {
	return "dummy"
}

func (d UploadDataInput) GetPointId() string {
	return d.PointId
}

func (d UploadDataInput) Origin() string {
	return origin.FindOrigin(d.PointId)
}
