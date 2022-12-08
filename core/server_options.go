package core

import (
	"crypto/tls"
	"net"

	"github.com/lucas-clemente/quic-go"
	"github.com/yomorun/yomo/core/auth"
	"github.com/yomorun/yomo/core/ylog"
	pkgtls "github.com/yomorun/yomo/pkg/tls"
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
	QuicConfig  *quic.Config
	TLSConfig   *tls.Config
	Addr        string
	Auths       map[string]auth.Authentication
	Conn        net.PacketConn
	logger      *slog.Logger
	alpnHandler func(proto string) error
}

func defaultServerOptions() *serverOptions {
	logger := ylog.Default()

	return &serverOptions{
		QuicConfig: DefalutQuicConfig,
		TLSConfig:  pkgtls.MustCreateClientTLSConfig(),
		Addr:       DefaultListenAddr,
		Auths:      map[string]auth.Authentication{},
		Conn:       nil,
		logger:     logger,
		alpnHandler: func(proto string) error {
			logger.Info("client alpn proto", "component", "server", "proto", proto)
			return nil
		},
	}
}

// WithAddr sets the server address.
func WithAddr(addr string) ServerOption {
	return func(o *serverOptions) {
		o.Addr = addr
	}
}

// WithAuth sets the server authentication method.
func WithAuth(name string, args ...string) ServerOption {
	return func(o *serverOptions) {
		if a, ok := auth.GetAuth(name); ok {
			a.Init(args...)
			if o.Auths == nil {
				o.Auths = make(map[string]auth.Authentication)
			}
			o.Auths[a.Name()] = a
		}
	}
}

// WithServerTLSConfig sets the TLS configuration for the server.
func WithServerTLSConfig(tc *tls.Config) ServerOption {
	return func(o *serverOptions) {
		o.TLSConfig = tc
	}
}

// WithServerQuicConfig sets the QUIC configuration for the server.
func WithServerQuicConfig(qc *quic.Config) ServerOption {
	return func(o *serverOptions) {
		o.QuicConfig = qc
	}
}

// WithConn sets the connection for the server.
func WithConn(conn net.PacketConn) ServerOption {
	return func(o *serverOptions) {
		o.Conn = conn
	}
}

// WithServerLogger sets the logger for the server.
func WithServerLogger(logger *slog.Logger) ServerOption {
	return func(o *serverOptions) {
		o.logger = logger
	}
}
