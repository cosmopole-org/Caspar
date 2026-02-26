package inputs_users

type MintInput struct {
	ToUserEmail string `json:"toUserEmail" validate:"required"`
	Amount      int64  `json:"amount" validate:"required"`
}

func (d MintInput) GetData() any {
	return "dummy"
}

func (d MintInput) GetPointId() string {
	return ""
}

func (d MintInput) Origin() string {
	return "global"
}
