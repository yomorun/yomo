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
	ErrFirstFrameIsNotHandshake = errors.New("the client didn't handshake immediately on connection")
	ErrHandshakeReadTimeout     = errors.New("the client handshake timeout")
)

type ControlStream struct {
	conn            quic.Connection
	group           sync.WaitGroup
	stream          quic.Stream
	logger          *slog.Logger
	metadataBuilder metadata.Builder
}

func NewControlStream(
	conn quic.Connection,
	stream quic.Stream,
	logger *slog.Logger,
	metadataBuilder metadata.Builder,
) *ControlStream {
	return &ControlStream{
		conn:            conn,
		stream:          stream,
		logger:          logger,
		metadataBuilder: metadataBuilder,
	}
}

func (cs *ControlStream) Handshake(timeout time.Duration, handshakeFunc func(*frame.HandshakeFrame) (bool, error)) error {
	errch := make(chan error)

	go func() {
		var gerr error
		defer func() { errch <- gerr }()

		first, err := ParseFrame(cs.stream)
		if err != nil {
			gerr = err
			return
		}

		f, ok := first.(*frame.HandshakeFrame)
		if !ok {
			gerr = ErrFirstFrameIsNotHandshake
			return
		}

		ok, err = handshakeFunc(f)
		if err != nil {
			gerr = cs.handshakeFailed(err)
			return
		}

		if !ok {
			errhandshake := fmt.Errorf("handshake authentication failed, client credential name is %s", f.AuthName())
			gerr = cs.handshakeFailed(errhandshake)
			return
		}

		gerr = cs.handshakeAck()
	}()

	select {
	case <-time.After(timeout):
		return ErrHandshakeReadTimeout
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

func (cs *ControlStream) runConn(
	connector Connector,
	runConnFunc func(c *Context),
) error {
	for {
		f, err := ParseFrame(cs.stream)
		if err != nil {
			return err
		}

		switch f.Type() {
		case frame.TagOfConnectionFrame:
			ff := f.(*frame.ConnectionFrame)
			stream, err := cs.conn.OpenStream()
			if err != nil {
				return err
			}

			metadata, err := cs.metadataBuilder.Build(ff)
			if err != nil {
				return err
			}

			// TODO: Connection and Context is almost identical.
			// TODO: conn should has ability to let other object to known if It is alive.
			// TODO: connector should scan conn to gc dead conn.
			conn := newConnection(ff.Name, ff.ClientID, ClientType(ff.ClientType), metadata, stream, ff.ObserveDataTags, cs.logger)
			connector.Add(conn.ClientID(), conn)
			cs.group.Add(1)

			go func() {
				defer cs.group.Done()
				// TODO: runConn function should accept a sign for exit.

				c := newContext(conn, stream, cs.metadataBuilder, cs.logger)
				defer c.Clean()

				runConnFunc(c)
			}()

		case frame.TagOfAcceptedFrame:
			ff := f.(*frame.ConnectionCloseFrame)

			conn := connector.Get(ff.ClientID)

			message := frame.NewHandshakeAckFrame()

			if err := conn.WriteFrame(message); err != nil {
				cs.logger.Warn("WriteFrame failed", "client_id", ff.ClientID)
				continue
			}
			// TODO: There should check if the conn is closed before call runConn.
			conn.Close()

			connector.Remove(conn.ClientID())
		}
	}

}

func (cs *ControlStream) Wait() { cs.group.Wait() }
