package inputs_storage

type DownloadInput struct {
	FileId  string `json:"fileId" validate:"required"`
	PointId string `json:"pointId" validate:"required"`
}

func (d DownloadInput) GetData() any {
	return "dummy"
}

func (d DownloadInput) GetPointId() string {
	return d.PointId
}

func (d DownloadInput) Origin() string {
	return ""
}
