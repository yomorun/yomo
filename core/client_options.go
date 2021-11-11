package core

import (
	"crypto/tls"

	"github.com/lucas-clemente/quic-go"
	"github.com/yomorun/yomo/core/auth"
)

type ClientOptions struct {
	QuicConfig *quic.Config
	TLSConfig  *tls.Config
	Credential auth.Credential
	AppID      string
}

func WithCredential(cred auth.Credential) ClientOption {
	return func(o *ClientOptions) {
		o.Credential = cred
	}
}

func WithClientTLSConfig(tc *tls.Config) ClientOption {
	return func(o *ClientOptions) {
		o.TLSConfig = tc
	}
}

func WithClientQuicConfig(qc *quic.Config) ClientOption {
	return func(o *ClientOptions) {
		o.QuicConfig = qc
	}
}

func WithAppID(appID string) ClientOption {
	return func(o *ClientOptions) {
		o.AppID = appID
	}
}
