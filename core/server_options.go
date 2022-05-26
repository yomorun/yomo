package core

import (
	"crypto/tls"
	"net"

	"github.com/lucas-clemente/quic-go"
	"github.com/yomorun/yomo/core/auth"
)

type ServerOptions struct {
	QuicConfig *quic.Config
	TLSConfig  *tls.Config
	Addr       string
	Auths      []auth.Authentication
	Conn       net.PacketConn
}

func WithAddr(addr string) ServerOption {
	return func(o *ServerOptions) {
		o.Addr = addr
	}
}

// WithAuth sets the server authentication method
func WithAuth(name string, args ...string) ServerOption {
	return func(o *ServerOptions) {
		if auth, ok := auth.GetAuth(name); ok {
			auth.Init(args...)
			o.Auths = append(o.Auths, auth)
		}
	}
}

func WithServerTLSConfig(tc *tls.Config) ServerOption {
	return func(o *ServerOptions) {
		o.TLSConfig = tc
	}
}

func WithServerQuicConfig(qc *quic.Config) ServerOption {
	return func(o *ServerOptions) {
		o.QuicConfig = qc
	}
}

func WithConn(conn net.PacketConn) ServerOption {
	return func(o *ServerOptions) {
		o.Conn = conn
	}
}
