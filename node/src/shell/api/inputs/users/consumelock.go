package inputs_users

type ConsumeLockInput struct {
	Type      string `json:"type" validate:"required"`
	UserId    string `json:"userId" validate:"required"`
	LockId    string `json:"lockId" validate:"required"`
	Signature string `json:"signature" validate:"required"`
	Amount    int64  `json:"amount" validate:"required"`
}

func (d ConsumeLockInput) GetData() any {
	return "dummy"
}

func (d ConsumeLockInput) GetPointId() string {
	return ""
}

func (d ConsumeLockInput) Origin() string {
	return "global"
}
