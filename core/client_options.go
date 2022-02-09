package core

import (
	"crypto/tls"

	"github.com/lucas-clemente/quic-go"
	"github.com/yomorun/yomo/core/auth"
)

type ClientOptions struct {
	// InstanceID is the unique id of the client.
	InstanceID string
	QuicConfig *quic.Config
	TLSConfig  *tls.Config
	Credential auth.Credential
}

// WithInstanceID sets client instance id.
func WithInstanceID(id string) ClientOption {
	return func(o *ClientOptions) {
		o.InstanceID = id
	}
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
