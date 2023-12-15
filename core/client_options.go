package core

import (
	"crypto/tls"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/yomorun/yomo/core/auth"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/ylog"
	pkgtls "github.com/yomorun/yomo/pkg/tls"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/exp/slog"
)

// ClientOption YoMo client options
type ClientOption func(*clientOptions)

// clientOptions are the options for YoMo client.
type clientOptions struct {
	observeDataTags []frame.Tag
	quicConfig      *quic.Config
	tlsConfig       *tls.Config
	credential      *auth.Credential
	reconnect       bool
	nonBlockWrite   bool
	logger          *slog.Logger
	tracerProvider  trace.TracerProvider
}

func defaultClientOption() *clientOptions {
	logger := ylog.Default()

	defaultQuicConfig := &quic.Config{
		Versions:                       []quic.VersionNumber{quic.Version1, quic.Version2},
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
		quicConfig:      defaultQuicConfig,
		tlsConfig:       pkgtls.MustCreateClientTLSConfig(),
		credential:      auth.NewCredential(""),
		logger:          logger,
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

// WithReConnect makes client Connect until success, unless authentication fails.
func WithReConnect() ClientOption {
	return func(o *clientOptions) {
		o.reconnect = true
	}
}

// WithNonBlockWrite makes client WriteFrame non-blocking.
func WithNonBlockWrite() ClientOption {
	return func(o *clientOptions) {
		o.nonBlockWrite = true
	}
}

// WithLogger sets logger for the client.
func WithLogger(logger *slog.Logger) ClientOption {
	return func(o *clientOptions) {
		o.logger = logger
	}
}

// WithTracerProvider sets tracer provider for the client.
func WithTracerProvider(tp trace.TracerProvider) ClientOption {
	return func(o *clientOptions) {
		o.tracerProvider = tp
	}
}
