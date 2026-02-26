package inputs_storage

type DownloadPointEntityInput struct {
	EntityId string `json:"entityId" validate:"required"`
	PointId  string `json:"pointId" validate:"required"`
}

func (d DownloadPointEntityInput) GetData() any {
	return "dummy"
}

func (d DownloadPointEntityInput) GetPointId() string {
	return d.PointId
}

func (d DownloadPointEntityInput) Origin() string {
	return ""
}
