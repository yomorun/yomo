package core

import (
	"context"
	"crypto/tls"
	"net"
	"time"

	"github.com/quic-go/quic-go"
	pkgtls "github.com/yomorun/yomo/pkg/tls"
	"golang.org/x/exp/slog"
)

// quicListener implements Listener interface.
type quicListener struct {
	underlying quic.Listener
}

var _ Listener = (*quicListener)(nil)

func (ql *quicListener) Addr() net.Addr { return ql.underlying.Addr() }
func (ql *quicListener) Close() error   { return ql.underlying.Close() }
func (ql *quicListener) Accept(ctx context.Context) (Connection, error) {
	qconn, err := ql.underlying.Accept(ctx)
	if err != nil {
		return nil, err
	}

	return &QuicConnection{qconn}, nil
}

// DefalutQuicConfig be used when `quicConfig` is nil.
var DefalutQuicConfig = &quic.Config{
	Versions:                       []quic.VersionNumber{quic.VersionDraft29, quic.Version1, quic.Version2},
	MaxIdleTimeout:                 time.Second * 5,
	KeepAlivePeriod:                time.Second * 2,
	MaxIncomingStreams:             1000,
	MaxIncomingUniStreams:          1000,
	HandshakeIdleTimeout:           time.Second * 3,
	InitialStreamReceiveWindow:     1024 * 1024 * 2,
	InitialConnectionReceiveWindow: 1024 * 1024 * 2,
	// DisablePathMTUDiscovery:        true,
}

// NewQuicListener returns quic Listener.
func NewQuicListener(conn net.PacketConn, tlsConfig *tls.Config, quicConfig *quic.Config, logger *slog.Logger) (Listener, error) {
	if tlsConfig == nil {
		tc, err := pkgtls.CreateServerTLSConfig(conn.LocalAddr().String())
		if err != nil {
			logger.Error("generate server tls config failed", err)
			return &quicListener{}, err
		}
		tlsConfig = tc
	}

	if quicConfig == nil {
		quicConfig = DefalutQuicConfig
	}

	ql, err := quic.Listen(conn, tlsConfig, quicConfig)
	if err != nil {
		return &quicListener{ql}, err
	}

	return &quicListener{ql}, nil
}

// QuicConnection implements Connection interface.
type QuicConnection struct {
	conn quic.Connection
}

// YomoCloseErrorCode is the error code for close quic Connection for yomo.
// If the Connection implemented by quic is closed, the quic ApplicationErrorCode is always 0x13.
const YomoCloseErrorCode = quic.ApplicationErrorCode(0x13)

// LocalAddr returns the local address.
func (qc *QuicConnection) LocalAddr() string {
	return qc.conn.LocalAddr().String()
}

// RemoteAddr returns the address of the peer.
func (qc *QuicConnection) RemoteAddr() string {
	return qc.conn.RemoteAddr().String()
}

// OpenStream opens a new bidirectional QUIC stream.
func (qc *QuicConnection) OpenStream() (ContextReadWriteCloser, error) {
	return qc.conn.OpenStream()
}

// AcceptStream returns the next stream opened by the peer, blocking until one is available.
func (qc *QuicConnection) AcceptStream(ctx context.Context) (ContextReadWriteCloser, error) {
	return qc.conn.AcceptStream(ctx)
}

// CloseWithError closes the connection with an error string.
func (qc *QuicConnection) CloseWithError(errString string) error {
	return qc.conn.CloseWithError(YomoCloseErrorCode, errString)
}
