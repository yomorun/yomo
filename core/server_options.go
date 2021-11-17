package core

import (
	"crypto/tls"
	"net"

	"github.com/lucas-clemente/quic-go"
	"github.com/yomorun/yomo/core/auth"
	"github.com/yomorun/yomo/core/store"
)

type ServerOptions struct {
	// Listener   Listener
	QuicConfig *quic.Config
	TLSConfig  *tls.Config
	Addr       string
	// TODO: 移到 BeforeHandshakeFrameHandler, 这个应该是个数组,以便同时支持不同的鉴权方式
	Auth  auth.Authentication
	Store store.Store
	Conn  net.PacketConn
	// TODO: 不在这里增加,直接增加Server方法
	// 增加 BeforeHandshakeFrameHandler
	// 增加 AfterHandshakeFrameHandler
	// 增加 BeforeDataFrameHandler
	// 增加 AfterDataFrameHandler
}

// func WithListener(l Listener) ServerOption {
// 	return func(o *ServerOptions) {
// 		o.Listener = l
// 	}
// }

func WithAddr(addr string) ServerOption {
	return func(o *ServerOptions) {
		o.Addr = addr
	}
}

func WithAuth(auth auth.Authentication) ServerOption {
	return func(o *ServerOptions) {
		// TODO: 追加方式
		o.Auth = auth
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
