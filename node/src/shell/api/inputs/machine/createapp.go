package inputs_machiner

type CreateAppInput struct {
	ChainId  string  `json:"chainId" validate:"required"`
	Username string `json:"username" validate:"required"`
	Metadata any    `json:"metadata" validate:"required"`
}

func (d CreateAppInput) GetData() any {
	return "dummy"
}

func (d CreateAppInput) GetPointId() string {
	return ""
}

func (d CreateAppInput) Origin() string {
	return "global"
}
