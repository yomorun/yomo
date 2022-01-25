package core

import (
	"crypto/tls"
	"net"
	"time"

	"github.com/lucas-clemente/quic-go"
	pkgtls "github.com/yomorun/yomo/pkg/tls"
)

var _ Listener = (*defaultListener)(nil)

type defaultListener struct {
	c *quic.Config
	quic.Listener
}

func newListener() *defaultListener {
	return &defaultListener{}
}

func (l *defaultListener) Name() string {
	return "QUIC-Server"
}

func (l *defaultListener) Listen(conn net.PacketConn, tlsConfig *tls.Config, quicConfig *quic.Config) error {
	var err error
	// tls config
	var tc *tls.Config = tlsConfig
	if tc == nil {
		tc, err = pkgtls.GetTLSConfig()
		if err != nil {
			return err
		}
	}
	// quic config
	var c *quic.Config = quicConfig
	if c == nil {
		c = &quic.Config{
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
	l.c = c

	listener, err := quic.Listen(conn, tc, l.c)
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
