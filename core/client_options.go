package core

import (
	"crypto/tls"

	"github.com/lucas-clemente/quic-go"
	"github.com/yomorun/yomo/core/auth"
	"github.com/yomorun/yomo/core/log"
)

type ClientOptions struct {
	ObserveDataTags []byte
	QuicConfig      *quic.Config
	TLSConfig       *tls.Config
	Credential      *auth.Credential
	Logger          log.Logger
}

// WithObserveDataTags sets data tag list for the client.
func WithObserveDataTags(tags ...byte) ClientOption {
	return func(o *ClientOptions) {
		o.ObserveDataTags = tags
	}
}

// WithCredential sets the client credential method (used by client)
func WithCredential(payload string) ClientOption {
	return func(o *ClientOptions) {
		o.Credential = auth.NewCredential(payload)
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

// WithLogger sets logger for the client.
func WithLogger(logger log.Logger) ClientOption {
	return func(o *ClientOptions) {
		o.Logger = logger
	}
}
