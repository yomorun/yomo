package core

import (
	"crypto/tls"

	"github.com/lucas-clemente/quic-go"
	"github.com/yomorun/yomo/internal/auth"
)

type ClientOptions struct {
	QuicConfig *quic.Config
	TLSConfig  *tls.Config
	Credential auth.Credential
}

func WithCredential(cred auth.Credential) ClientOption {
	return func(o *ClientOptions) {
		o.Credential = cred
	}
}
