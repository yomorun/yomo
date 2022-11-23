package core

import (
	"crypto/tls"
	"net"

	"github.com/lucas-clemente/quic-go"
	"github.com/yomorun/yomo/core/auth"
)

// ServerOptions are the options for YoMo server.
type ServerOptions struct {
	QuicConfig *quic.Config
	TLSConfig  *tls.Config
	Addr       string
	Auths      map[string]auth.Authentication
	Conn       net.PacketConn
}

// WithAddr sets the server address.
func WithAddr(addr string) ServerOption {
	return func(o *ServerOptions) {
		o.Addr = addr
	}
}

// WithAuth sets the server authentication method.
func WithAuth(name string, args ...string) ServerOption {
	return func(o *ServerOptions) {
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
	return func(o *ServerOptions) {
		o.TLSConfig = tc
	}
}

// WithServerQuicConfig sets the QUIC configuration for the server.
func WithServerQuicConfig(qc *quic.Config) ServerOption {
	return func(o *ServerOptions) {
		o.QuicConfig = qc
	}
}

// WithConn sets the connection for the server.
func WithConn(conn net.PacketConn) ServerOption {
	return func(o *ServerOptions) {
		o.Conn = conn
	}
}
