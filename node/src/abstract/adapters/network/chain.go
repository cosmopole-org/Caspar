package network

import "crypto/tls"

type IChain interface {
	Listen(port int, tlsConfig *tls.Config)
	SubmitTrx(chainId string, machineId string, typ string, payload []byte)
	RegisterPipeline(pipeline func([][]byte, func([]byte)) []string)
	NotifyNewMachineCreated(chainId string, machineId string)
	CreateTempChain() string
	CreateWorkChain() string
	Peers() []string
	UserOwnsOrigin(userId string, origin string) bool
	GetNodeOwnerId(origin string) string
	GetValidatorsOfMachineShard(machineId string) []string
	Close()
}
