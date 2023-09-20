package core

import (
	"crypto/tls"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/yomorun/yomo/core/auth"
	"github.com/yomorun/yomo/core/ylog"
	oteltrace "go.opentelemetry.io/otel/trace"
	"golang.org/x/exp/slog"
)

// DefalutQuicConfig be used when `quicConfig` is nil.
var DefalutQuicConfig = &quic.Config{
	Versions:                       []quic.VersionNumber{quic.Version1, quic.Version2},
	MaxIdleTimeout:                 time.Second * 5,
	KeepAlivePeriod:                time.Second * 2,
	MaxIncomingStreams:             1000,
	MaxIncomingUniStreams:          1000,
	HandshakeIdleTimeout:           time.Second * 3,
	InitialStreamReceiveWindow:     1024 * 1024 * 2,
	InitialConnectionReceiveWindow: 1024 * 1024 * 2,
	// DisablePathMTUDiscovery:        true,
}

// ServerOption is the option for server.
type ServerOption func(*serverOptions)

// ServerOptions are the options for YoMo server.
// TODO: quic alpn function.
type serverOptions struct {
	quicConfig     *quic.Config
	tlsConfig      *tls.Config
	auths          map[string]auth.Authentication
	logger         *slog.Logger
	tracerProvider oteltrace.TracerProvider
}

func defaultServerOptions() *serverOptions {
	logger := ylog.Default()

	opts := &serverOptions{
		quicConfig: DefalutQuicConfig,
		tlsConfig:  nil,
		auths:      map[string]auth.Authentication{},
		logger:     logger,
	}
	return opts
}

// WithAuth sets the server authentication method.
func WithAuth(name string, args ...string) ServerOption {
	return func(o *serverOptions) {
		if a, ok := auth.GetAuth(name); ok {
			a.Init(args...)
			if o.auths == nil {
				o.auths = make(map[string]auth.Authentication)
			}
			o.auths[a.Name()] = a
		}
	}
}

// WithServerTLSConfig sets the TLS configuration for the server.
func WithServerTLSConfig(tc *tls.Config) ServerOption {
	return func(o *serverOptions) {
		o.tlsConfig = tc
	}
}

// WithServerQuicConfig sets the QUIC configuration for the server.
func WithServerQuicConfig(qc *quic.Config) ServerOption {
	return func(o *serverOptions) {
		o.quicConfig = qc
	}
}

// WithServerLogger sets logger for the server.
func WithServerLogger(logger *slog.Logger) ServerOption {
	return func(o *serverOptions) {
		o.logger = logger
	}
}

// WithServerTracerProvider sets tracer provider for the server.
func WithServerTracerProvider(tp oteltrace.TracerProvider) ServerOption {
	return func(o *serverOptions) {
		o.tracerProvider = tp
	}
}
