package core

import (
	"context"
	"crypto/tls"
	"time"

	"github.com/lucas-clemente/quic-go"
	pkgtls "github.com/yomorun/yomo/pkg/tls"
)

const (
	DefaultListenAddr = "0.0.0.0:9000"
)

type defaultListener struct {
	tc *tls.Config
	c  *quic.Config
	quic.Listener
}

func newListener(tlsConfig *tls.Config, quicConfig *quic.Config) *defaultListener {
	return &defaultListener{
		tc: tlsConfig,
		c:  quicConfig,
	}
}

func (l *defaultListener) Name() string {
	return "QUIC-Server"
}

func (l *defaultListener) Listen(ctx context.Context, addr string) error {
	// listen addr
	if addr == "" {
		addr = DefaultListenAddr
	}
	// tls config
	if l.tc == nil {
		l.tc = pkgtls.GenerateTLSConfig(addr)
	}
	// quic config
	if l.c == nil {
		l.c = &quic.Config{
			Versions:                       []quic.VersionNumber{quic.Version1, quic.VersionDraft29},
			MaxIdleTimeout:                 time.Second * 5,
			KeepAlive:                      true,
			MaxIncomingStreams:             1000,
			MaxIncomingUniStreams:          1000,
			HandshakeIdleTimeout:           time.Second * 3,
			InitialStreamReceiveWindow:     1024 * 1024 * 2,
			InitialConnectionReceiveWindow: 1024 * 1024 * 2,
			DisablePathMTUDiscovery:        true,
			// Tracer:                         getQlogConfig("server"),
		}
	}

	listener, err := quic.ListenAddr(addr, l.tc, l.c)
	if err != nil {
		return err
	}
	l.Listener = listener
	return nil
}

func (l *defaultListener) Versions() []string {
	vers := make([]string, 0)
	for _, v := range l.c.Versions {
		vers = append(vers, v.String())
	}
	return vers
}
