package network

import "crypto/tls"

type INetwork interface {
	Chain() IChain
	Federation() IFederation
	Tcp() ITcp
	Ws() IWs
	TlsConfig() *tls.Config
	Run(ports map[string]int)
}
