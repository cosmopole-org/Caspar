package inputs_storage

import (
	"mime/multipart"
)

type UploadInput struct {
	Data    *multipart.FileHeader `json:"data" validate:"required"`
	PointId string                `json:"pointId" validate:"required"`
	FileId  string                `json:"fileId"`
}

func (d UploadInput) GetData() any {
	return "dummy"
}

func (d UploadInput) GetPointId() string {
	return d.PointId
}

func (d UploadInput) Origin() string {
	return ""
}
