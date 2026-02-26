package network

import "crypto/tls"

type ITcp interface {
	Listen(port int, tlsConfig *tls.Config)
}
