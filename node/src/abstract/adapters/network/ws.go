package network

import "crypto/tls"

type IWs interface {
	Listen(port int, tlsConfig *tls.Config)
}
