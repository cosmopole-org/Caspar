package action

import (
	"kasper/src/abstract/models/input"
	"kasper/src/abstract/models/trx"
	"kasper/src/abstract/state"
)

type IActions interface {
	Install(state.IState, ...any)
}

type IAction interface {
	StateModifier() func(bool, func(trx.ITrx) error)
	Key() string
	Act(state.IState, input.IInput) (int, any, error)
}

type ISecureAction interface {
	Key() string
	HasGlobalParser() bool
	ParseInput(protocol string, raw interface{}) (input.IInput, error)
	SecurelyAct(userId string, packetId string, packetBinary []byte, packetSignature string, input input.IInput, dummy string, insider ...bool) (int, any, error)
	SecurlyActChain(userId string, packetId string, packetBinary []byte, packetSignature string, input input.IInput, origin string, tag string)
	SecurelyActFed(userId string, packetBinary []byte, packetSignature string, input input.IInput) (int, any, error)
}

type ExtendedField struct {
	Name        string
	Path        string
	Type        string
	Default     any
	Required    bool
	Searchable  bool
	PrimaryProp bool
}
