package inputs_machiner

type SubBaseTrxInput struct {
	ChainId   int64  `json:"chainId" validate:"required"`
	Key       string `json:"key" validate:"required"`
	Payload   []byte `json:"payload" validate:"required"`
	Signature string `json:"signature" validate:"required"`
}

func (d SubBaseTrxInput) GetData() any {
	return "dummy"
}

func (d SubBaseTrxInput) GetPointId() string {
	return ""
}

func (d SubBaseTrxInput) Origin() string {
	return "global"
}
