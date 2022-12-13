package core

import (
	"crypto/tls"
	"time"

	"github.com/lucas-clemente/quic-go"
	"github.com/yomorun/yomo/core/auth"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/ylog"
	pkgtls "github.com/yomorun/yomo/pkg/tls"
	"golang.org/x/exp/slog"
)

// clientOptions are the options for YoMo client.
type clientOptions struct {
	observeDataTags []frame.Tag
	quicConfig      *quic.Config
	tlsConfig       *tls.Config
	credential      *auth.Credential
	logger          *slog.Logger
}

func defaultClientOption() *clientOptions {
	logger := ylog.Default()

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

	opts := &clientOptions{
		observeDataTags: make([]frame.Tag, 0),
		quicConfig:      defalutQuicConfig,
		tlsConfig:       pkgtls.MustCreateClientTLSConfig(),
		credential:      auth.NewCredential(""),
		logger:          logger,
	}

	if opts.credential != nil {
		logger.Info("use credential", "component", "client", "credential_name", opts.credential.Name())
	}

	return opts
}

// WithObserveDataTags sets data tag list for the client.
func WithObserveDataTags(tags ...frame.Tag) ClientOption {
	return func(o *clientOptions) {
		o.observeDataTags = tags
	}
}

// WithCredential sets the client credential method (used by client).
func WithCredential(payload string) ClientOption {
	return func(o *clientOptions) {
		o.credential = auth.NewCredential(payload)
	}
}

// WithClientTLSConfig sets tls config for the client.
func WithClientTLSConfig(tc *tls.Config) ClientOption {
	return func(o *clientOptions) {
		if tc != nil {
			o.tlsConfig = tc
		}
	}
}

// WithClientQuicConfig sets quic config for the client.
func WithClientQuicConfig(qc *quic.Config) ClientOption {
	return func(o *clientOptions) {
		o.quicConfig = qc
	}
}

// WithLogger sets logger for the client.
func WithLogger(logger *slog.Logger) ClientOption {
	return func(o *clientOptions) {
		o.logger = logger
	}
}
