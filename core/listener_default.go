package core

import (
	"crypto/tls"
	"net"
	"time"

	"github.com/quic-go/quic-go"
	pkgtls "github.com/yomorun/yomo/pkg/tls"
	"golang.org/x/exp/slog"
)

var _ Listener = (*defaultListener)(nil)

type defaultListener struct {
	quic.Listener
	conf *quic.Config
}

// DefalutQuicConfig be used when `quicConfig` is nil.
var DefalutQuicConfig = &quic.Config{
	Versions:                       []quic.VersionNumber{quic.Version2, quic.Version1, quic.VersionDraft29},
	MaxIdleTimeout:                 time.Second * 5,
	KeepAlivePeriod:                time.Second * 2,
	MaxIncomingStreams:             1000,
	MaxIncomingUniStreams:          1000,
	HandshakeIdleTimeout:           time.Second * 3,
	InitialStreamReceiveWindow:     1024 * 1024 * 2,
	InitialConnectionReceiveWindow: 1024 * 1024 * 2,
	// DisablePathMTUDiscovery:        true,
}

func newListener(conn net.PacketConn, tlsConfig *tls.Config, quicConfig *quic.Config, logger *slog.Logger) (*defaultListener, error) {
	if tlsConfig == nil {
		tc, err := pkgtls.CreateServerTLSConfig(conn.LocalAddr().String())
		if err != nil {
			logger.Error("CreateServerTLSConfig error", err)
			return &defaultListener{}, err
		}
		tlsConfig = tc
	}

	if quicConfig == nil {
		quicConfig = DefalutQuicConfig
	}

	quicListener, err := quic.Listen(conn, tlsConfig, quicConfig)
	if err != nil {
		logger.Error("quic Listen error", err)
		return &defaultListener{}, err
	}

	return &defaultListener{conf: quicConfig, Listener: quicListener}, nil
}

func (l *defaultListener) Name() string { return "QUIC-Server" }

func (l *defaultListener) Versions() []string {
	versions := make([]string, len(l.conf.Versions))
	for k, v := range l.conf.Versions {
		versions[k] = v.String()
	}
	return versions
}
