package core

import (
	"crypto/tls"

	"github.com/lucas-clemente/quic-go"
	"github.com/yomorun/yomo/internal/auth"
	"github.com/yomorun/yomo/internal/store"
)

type ServerOptions struct {
	Listener   Listener
	QuicConfig *quic.Config
	TLSConfig  *tls.Config
	Addr       string
	Auth       auth.Authentication
	Store      store.Store
}

func WithListener(l Listener) ServerOption {
	return func(o *ServerOptions) {
		o.Listener = l
	}
}

func WithAddr(addr string) ServerOption {
	return func(o *ServerOptions) {
		o.Addr = addr
	}
}

func WithAuth(auth auth.Authentication) ServerOption {
	return func(o *ServerOptions) {
		o.Auth = auth
	}
}

func WithStore(store store.Store) ServerOption {
	return func(o *ServerOptions) {
		o.Store = store
	}
}
