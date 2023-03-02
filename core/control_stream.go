package core

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
	"golang.org/x/exp/slog"
)

var (
	// ErrFirstFrameIsNotAuthentication be returned if the first frame accepted by control stream is not AuthenticationFrame.
	ErrFirstFrameIsNotAuthentication = errors.New("yomo: client didn't authenticate immediately on connection")

	// ErrAuthenticateTimeout be returned if server don't receive authentication ack.
	ErrAuthenticateTimeout = errors.New("yomo: client authenticate timeout")
)

// ControlStream is the stream to control other DataStream.
// One Connection has many DataStream and only one ControlStream, ControlStream authenticates
// Connection and recevies HandshakeFrame and CloseStreamFrame to create DataStream and close
// stream. the ControlStream always the first stream established between server and client.
type ControlStream struct {
	conn            quic.Connection
	group           sync.WaitGroup
	stream          quic.Stream
	logger          *slog.Logger
	metadataBuilder metadata.Builder
}

// NewControlStream returns ControlStream.
func NewControlStream(conn quic.Connection, stream quic.Stream, logger *slog.Logger) *ControlStream {
	return &ControlStream{
		conn:   conn,
		stream: stream,
		logger: logger,
	}
}

// Auth authenticates client in authFunc
func (cs *ControlStream) Auth(timeout time.Duration, authFunc func(*frame.AuthenticationFrame) (bool, error)) error {
	errch := make(chan error)

	go func() {
		var gerr error
		defer func() { errch <- gerr }()

		first, err := ParseFrame(cs.stream)
		if err != nil {
			gerr = err
			return
		}

		f, ok := first.(*frame.AuthenticationFrame)
		if !ok {
			gerr = ErrFirstFrameIsNotAuthentication
			return
		}

		ok, err = authFunc(f)
		if err != nil {
			gerr = cs.handshakeFailed(err)
			return
		}

		if !ok {
			errhandshake := fmt.Errorf("yomo: authentication failed, client credential name is %s", f.AuthName())
			gerr = cs.handshakeFailed(errhandshake)
			return
		}

		gerr = cs.handshakeAck()
	}()

	select {
	case <-time.After(timeout):
		return ErrAuthenticateTimeout
	case err := <-errch:
		return err
	}
}

func (cs *ControlStream) handshakeFailed(se error) error {
	goaway := frame.NewGoawayFrame(se.Error())

	_, err := cs.stream.Write(goaway.Encode())
	if err != nil {
		return err
	}

	err = cs.conn.CloseWithError(quic.ApplicationErrorCode(0), se.Error())

	return err
}

func (cs *ControlStream) handshakeAck() error {
	ack := frame.NewHandshakeAckFrame()

	_, err := cs.stream.Write(ack.Encode())

	return err

}

func (cs *ControlStream) runConn(connector Connector, runConnFunc func(c *Context)) error {
	for {
		f, err := ParseFrame(cs.stream)
		if err != nil {
			return err
		}

		switch ff := f.(type) {
		case *frame.HandshakeFrame:
			stream, err := cs.conn.OpenStream()
			if err != nil {
				return err
			}
			stream.Write(frame.NewHandshakeAckFrame().Encode())

			conn := newConnection(ff.Name(), ff.ID(), ClientType(ff.StreamType()), nil, stream, ff.ObserveDataTags(), cs.logger)
			connector.Add(conn.ClientID(), conn)
			cs.group.Add(1)

			go func() {
				defer cs.group.Done()

				c, err := newContext(conn, stream, cs.metadataBuilder, cs.logger)
				if err != nil {
					c.conn.WriteFrame(frame.NewGoawayFrame(err.Error()))
				}
				defer c.Clean()

				runConnFunc(c)
			}()

			// TODO: close connection should be controled by controlStream
		}
	}

}

// Wait waits all dataStream down.
func (cs *ControlStream) Wait() { cs.group.Wait() }
