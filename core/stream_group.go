package core

import (
	"errors"
	"fmt"
	"sync"

	"github.com/quic-go/quic-go"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/core/yerr"
	"golang.org/x/exp/slog"
)

var (
	// ErrFirstFrameIsNotAuthentication be returned if the first frame accepted by control stream is not AuthenticationFrame.
	ErrFirstFrameIsNotAuthentication = errors.New("yomo: client didn't authenticate immediately on connection")

	// ErrAuthenticateTimeout be returned if server don't receive authentication ack.
	ErrAuthenticateTimeout = errors.New("yomo: client authenticate timeout")
)

// StreamGroup is the group of stream includes ControlStream amd DataStream.
// One Connection has many DataStream and only one ControlStream, ControlStream authenticates
// Connection and recevies HandshakeFrame and CloseStreamFrame to create DataStream or close
// stream. the ControlStream always the first stream established between server and client.
type StreamGroup struct {
	conn          quic.Connection
	group         sync.WaitGroup
	controlStream frame.ReadWriter
	logger        *slog.Logger
}

// NewStreamGroup returns StreamGroup.
func NewStreamGroup(conn quic.Connection, controlStream frame.ReadWriter, logger *slog.Logger) *StreamGroup {
	return &StreamGroup{
		conn:          conn,
		controlStream: controlStream,
		logger:        logger,
	}
}

// Auth authenticates client in authFunc.
func (g *StreamGroup) Auth(authFunc func(*frame.AuthenticationFrame) (bool, error)) error {
	first, err := g.controlStream.ReadFrame()
	if err != nil {
		return err
	}
	f, ok := first.(*frame.AuthenticationFrame)
	if !ok {
		return err
	}
	ok, err = authFunc(f)
	if err != nil {
		return err
	}
	if !ok {
		errhandshake := fmt.Errorf("yomo: authentication failed, client credential name is %s", f.AuthName())
		return g.handshakeFailed(errhandshake)
	}
	return g.handshakeAck()
}

func (g *StreamGroup) handshakeFailed(se error) error {
	ack := frame.NewAuthenticationAckFrame(false, se.Error())

	err := g.controlStream.WriteFrame(ack)
	if err != nil {
		return err
	}

	err = g.conn.CloseWithError(quic.ApplicationErrorCode(yerr.ErrorCodeRejected), se.Error())

	return err
}

func (g *StreamGroup) handshakeAck() error {
	ack := frame.NewAuthenticationAckFrame(true, "")
	return g.controlStream.WriteFrame(ack)
}

func (g *StreamGroup) run(connector Connector, contextFunc func(c *Context)) error {
	for {
		f, err := g.controlStream.ReadFrame()
		if err != nil {
			return err
		}

		switch ff := f.(type) {
		case *frame.HandshakeFrame:
			stream, err := g.conn.OpenStream()
			if err != nil {
				return err
			}
			stream.Write(frame.NewHandshakeAckFrame().Encode())

			dataStream := newDataStream(
				ff.Name(),
				ff.ID(),
				StreamType(ff.StreamType()),
				&metadata.Default{},
				stream,
				ff.ObserveDataTags(),
				g.logger,
			)
			connector.Add(dataStream.ID(), dataStream)
			g.group.Add(1)

			go func() {
				defer g.group.Done()

				c, err := newContext(g.controlStream, dataStream, g.logger)
				if err != nil {
					c.DataStream.WriteFrame(frame.NewGoawayFrame(err.Error()))
				}
				defer c.Clean()

				contextFunc(c)
			}()

		case *frame.CloseStreamFrame:
			ff.Reason()
			ff.StreamID()
		}
	}

}

// Wait waits all dataStream down.
func (g *StreamGroup) Wait() { g.group.Wait() }
