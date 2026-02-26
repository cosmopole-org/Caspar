package worker

type Trx struct {
	Key        string `json:"key"`
	Payload    string `json:"payload"`
	Signature  string `json:"signature"`
	UserId     string `json:"userId"`
	MachineId  string `json:"machineId"`
	Runtime    string `json:"runtime"`
	GasLimit   int64  `json:"gasLimit"`
	CallbackId string `json:"callbackId"`
}
