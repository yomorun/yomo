package core

import (
	"crypto/tls"

	"github.com/lucas-clemente/quic-go"
	"github.com/yomorun/yomo/core/auth"
)

type ClientOptions struct {
	ObservedDataTags []byte
	QuicConfig       *quic.Config
	TLSConfig        *tls.Config
	Credential       auth.Credential
}

// WithObservedDataTags sets data tag list for the client.
func WithObservedDataTags(tags ...byte) ClientOption {
	return func(o *ClientOptions) {
		o.ObservedDataTags = tags
	}
}

// WithCredential sets app auth for the client.
func WithCredential(cred auth.Credential) ClientOption {
	return func(o *ClientOptions) {
		o.Credential = cred
	}
}

// WithClientTLSConfig sets tls config for the client.
func WithClientTLSConfig(tc *tls.Config) ClientOption {
	return func(o *ClientOptions) {
		o.TLSConfig = tc
	}
}

// WithClientQuicConfig sets quic config for the client.
func WithClientQuicConfig(qc *quic.Config) ClientOption {
	return func(o *ClientOptions) {
		o.QuicConfig = qc
	}
}
