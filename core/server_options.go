package core

import (
	"crypto/tls"

	"github.com/quic-go/quic-go"
	"github.com/yomorun/yomo/core/auth"
	"github.com/yomorun/yomo/core/ylog"
	"golang.org/x/exp/slog"
)

const (
	// DefaultListenAddr is the default address to listen.
	DefaultListenAddr = "0.0.0.0:9000"
)

// ServerOption is the option for server.
type ServerOption func(*serverOptions)

// ServerOptions are the options for YoMo server.
type serverOptions struct {
	quicConfig  *quic.Config
	tlsConfig   *tls.Config
	addr        string
	auths       map[string]auth.Authentication
	logger      *slog.Logger
	alpnHandler func(proto string) error
}

func defaultServerOptions() *serverOptions {
	logger := ylog.Default()

	opts := &serverOptions{
		quicConfig: DefalutQuicConfig,
		tlsConfig:  nil,
		addr:       DefaultListenAddr,
		auths:      map[string]auth.Authentication{},
		logger:     logger,
	}
	opts.alpnHandler = func(proto string) error {
		opts.logger.Info("client alpn proto", "component", "server", "proto", proto)
		return nil
	}
	return opts
}

// WithAddr sets the server address.
func WithAddr(addr string) ServerOption {
	return func(o *serverOptions) {
		o.addr = addr
	}
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
