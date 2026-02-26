package core

import (
	"kasper/src/abstract/adapters/tools"
	"kasper/src/abstract/models/action"
	"kasper/src/abstract/models/chain"
	"kasper/src/abstract/models/info"
	"kasper/src/abstract/models/trx"
	"kasper/src/abstract/models/update"
	"kasper/src/abstract/state"
)

type EmptyPayload struct{}

type ResponseHolder struct {
	Payload []byte
	Effects chain.Effects
}

type ICore interface {
	OwnerId() string
	Id() string
	Gods() []string
	Tools() tools.ITools
	Executors() map[string]bool
	SetExecutors(map[string]bool)
	Actor() action.IActor
	Load([]string, map[string]interface{})
	Close()
	PlantChainTrigger(count int, userId string, tag string, machineId string, pointId string, input string)
	ExecAppletRequestOnChain(pointId string, machineId string, key string, payload []byte, signature string, userId string, tag string, tokenId string, callback func([]byte, int, error))
	ExecAppletResponseOnChain(callbackId string, packet []byte, signature string, resCode int, e string, updates []update.Update)
	ExecBaseRequestOnChain(key string, payload []byte, signature string, userId string, tag string, callback func([]byte, int, error))
	ExecBaseResponseOnChain(callbackId string, packet []byte, signature string, resCode int, e string, updates []update.Update, tag string, toUserId string)
	OnChainPacket(typ string, trxPayload []byte) string
	AppPendingTrxs()
	IpAddr() string
	ModifyState(bool, func(trx.ITrx) error)
	ModifyStateSecurlyWithSource(readonly bool, info info.IInfo, src string, fn func(state.IState) error)
	ModifyStateSecurly(readonly bool, info info.IInfo, fn func(state.IState) error)
	SignPacket(data []byte) string
	SignPacketAsOwner(data []byte) string
}
