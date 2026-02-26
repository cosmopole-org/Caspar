package update

type Update struct {
	Typ string `json:"type"`
	Key string `json:"key"`
	Val []byte `json:"val"`
}
