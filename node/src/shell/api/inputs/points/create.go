package inputs_points

type CreateInput struct {
	Tag      string          `json:"tag" validate:"required"`
	IsPublic *bool           `json:"isPublic" validate:"required"`
	PersHist *bool           `json:"persHist" validate:"required"`
	ParentId string          `json:"parentId"`
	Orig     string          `json:"orig"`
	Metadata any             `json:"metadata"`
	Members  map[string]bool `json:"members"`
}

func (d CreateInput) GetData() any {
	return "dummy"
}

func (d CreateInput) GetPointId() string {
	return ""
}

func (d CreateInput) Origin() string {
	return d.Orig
}
