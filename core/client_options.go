package core

import (
	"crypto/tls"
	"time"

	"github.com/lucas-clemente/quic-go"
	"github.com/yomorun/yomo/core/auth"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/log"
	"github.com/yomorun/yomo/pkg/logger"
	pkgtls "github.com/yomorun/yomo/pkg/tls"
)

// ClientOptions are the options for YoMo client.
type ClientOptions struct {
	ObserveDataTags []frame.Tag
	QuicConfig      *quic.Config
	TLSConfig       *tls.Config
	Credential      *auth.Credential
	Logger          log.Logger
}

func defaultClientOption() *ClientOptions {
	logger := logger.Default()

	defalutQuicConfig := &quic.Config{
		Versions:                       []quic.VersionNumber{quic.Version2},
		MaxIdleTimeout:                 time.Second * 40,
		KeepAlivePeriod:                time.Second * 20,
		MaxIncomingStreams:             1000,
		MaxIncomingUniStreams:          1000,
		HandshakeIdleTimeout:           time.Second * 3,
		InitialStreamReceiveWindow:     1024 * 1024 * 2,
		InitialConnectionReceiveWindow: 1024 * 1024 * 2,
		TokenStore:                     quic.NewLRUTokenStore(10, 5),
	}

	opts := &ClientOptions{
		ObserveDataTags: make([]frame.Tag, 0),
		QuicConfig:      defalutQuicConfig,
		TLSConfig:       pkgtls.MustCreateClientTLSConfig(),
		Credential:      auth.NewCredential(""),
		Logger:          logger,
	}

	// credential
	if opts.Credential != nil {
		logger.Printf("%suse credential: [%s]", ClientLogPrefix, opts.Credential.Name())
	}

	return opts
}

// WithObserveDataTags sets data tag list for the client.
func WithObserveDataTags(tags ...frame.Tag) ClientOption {
	return func(o *ClientOptions) {
		o.ObserveDataTags = tags
	}
}

// WithCredential sets the client credential method (used by client).
func WithCredential(payload string) ClientOption {
	return func(o *ClientOptions) {
		o.Credential = auth.NewCredential(payload)
	}
}

// WithClientTLSConfig sets tls config for the client.
func WithClientTLSConfig(tc *tls.Config) ClientOption {
	return func(o *ClientOptions) {
		if tc != nil {
			o.TLSConfig = tc
		}
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
