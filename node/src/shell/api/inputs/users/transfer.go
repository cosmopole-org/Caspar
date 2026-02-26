package inputs_users

type TransferInput struct {
	ToUsername string `json:"toUsername" validate:"required"`
	Amount     int64  `json:"amount" validate:"required"`
}

func (d TransferInput) GetData() any {
	return "dummy"
}

func (d TransferInput) GetPointId() string {
	return ""
}

func (d TransferInput) Origin() string {
	return "global"
}
