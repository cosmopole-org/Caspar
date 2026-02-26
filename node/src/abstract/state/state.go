package state

import (
	"kasper/src/abstract/models/info"
	"kasper/src/abstract/models/trx"
)

type IState interface {
	Info() info.IInfo
	Trx() trx.ITrx
	SetTrx(trx.ITrx)
	Source() string
	SetSource(string)
}
