package core

import (
	"crypto/tls"
	"net"

	"github.com/lucas-clemente/quic-go"
	"github.com/yomorun/yomo/core/auth"
	"github.com/yomorun/yomo/core/store"
)

type ServerOptions struct {
	QuicConfig *quic.Config
	TLSConfig  *tls.Config
	Addr       string
	Auths      []auth.Authentication
	Store      store.Store
	Conn       net.PacketConn
}

func WithAddr(addr string) ServerOption {
	return func(o *ServerOptions) {
		o.Addr = addr
	}
}

// func WithAuth(auth auth.Authentication) ServerOption {
// 	return func(o *ServerOptions) {
// 		o.Auths = append(o.Auths, auth)
// 	}
// }

func WithAuth(name string, args ...string) ServerOption {
	return func(o *ServerOptions) {
		if auth, ok := auth.GetAuth(name); ok {
			auth.Init(args...)
			o.Auths = append(o.Auths, auth)
		}
	}
}

func WithStore(store store.Store) ServerOption {
	return func(o *ServerOptions) {
		o.Store = store
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
