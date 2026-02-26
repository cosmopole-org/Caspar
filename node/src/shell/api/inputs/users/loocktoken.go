package inputs_users

type LockTokenInput struct {
	Amount int64  `json:"amount" validate:"required"`
	Type   string `json:"type" validate:"required"`
	Target string `json:"target" validate:"required"`
}

func (d LockTokenInput) GetData() any {
	return "dummy"
}

func (d LockTokenInput) GetPointId() string {
	return ""
}

func (d LockTokenInput) Origin() string {
	return "global"
}
