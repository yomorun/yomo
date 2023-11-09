// Package core provides the core functions of YoMo.
package core

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"time"

	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/pkg/frame-codec/y3codec"
	"github.com/yomorun/yomo/pkg/id"
	yquic "github.com/yomorun/yomo/pkg/listener/quic"
	oteltrace "go.opentelemetry.io/otel/trace"
	"golang.org/x/exp/slog"
)

// Client is the abstraction of a YoMo-Client. a YoMo-Client can be
// Source, Upstream Zipper or StreamFunction.
type Client struct {
	zipperAddr     string
	name           string                     // name of the client
	clientID       string                     // id of the client
	clientType     ClientType                 // type of the client
	processor      func(*frame.DataFrame)     // function to invoke when data arrived
	receiver       func(*frame.BackflowFrame) // function to invoke when data is processed
	errorfn        func(error)                // function to invoke when error occured
	opts           *clientOptions
	Logger         *slog.Logger
	tracerProvider oteltrace.TracerProvider

	// ctx and ctxCancel manage the lifecycle of client.
	ctx       context.Context
	ctxCancel context.CancelCauseFunc

	writeFrameChan chan frame.Frame
}

// NewClient creates a new YoMo-Client.
func NewClient(appName, zipperAddr string, clientType ClientType, opts ...ClientOption) *Client {
	option := defaultClientOption()

	for _, o := range opts {
		o(option)
	}
	clientID := id.New()

	logger := option.logger

	ctx, ctxCancel := context.WithCancelCause(context.Background())

	return &Client{
		zipperAddr:     zipperAddr,
		name:           appName,
		clientID:       clientID,
		processor:      func(df *frame.DataFrame) { logger.Warn("the processor has not been set") },
		receiver:       func(bf *frame.BackflowFrame) { logger.Warn("the receiver has not been set") },
		clientType:     clientType,
		opts:           option,
		Logger:         logger,
		tracerProvider: option.tracerProvider,
		errorfn:        func(err error) { logger.Error("client err", "err", err) },
		writeFrameChan: make(chan frame.Frame),
		ctx:            ctx,
		ctxCancel:      ctxCancel,
	}
}

func (c *Client) connect(ctx context.Context, addr string) (frame.Conn, error) {
	conn, err := yquic.DialAddr(ctx, addr, y3codec.Codec(), y3codec.PacketReadWriter(), c.opts.tlsConfig, c.opts.quicConfig)
	if err != nil {
		return conn, err
	}

	hf := &frame.HandshakeFrame{
		Name:            c.name,
		ID:              c.clientID,
		ClientType:      byte(c.clientType),
		ObserveDataTags: c.opts.observeDataTags,
		AuthName:        c.opts.credential.Name(),
		AuthPayload:     c.opts.credential.Payload(),
	}

	if err := conn.WriteFrame(hf); err != nil {
		return conn, err
	}

	received, err := conn.ReadFrame()
	if err != nil {
		return nil, err
	}
	switch received.Type() {
	case frame.TypeRejectedFrame:
		return conn, ErrAuthenticateFailed{received.(*frame.RejectedFrame).Message}
	case frame.TypeHandshakeAckFrame:
		return conn, nil
	default:
		return conn, ErrAuthenticateFailed{
			fmt.Sprintf("authentication failed: read unexcepted frame, frame read: %s", received.Type().String()),
		}
	}
}

func (c *Client) runBackground(ctx context.Context, conn frame.Conn) {
	reconnection := make(chan struct{})

	go c.handleReadFrames(conn, reconnection)

	var err error
	for {
		select {
		case <-c.ctx.Done():
			conn.CloseWithError("yomo: client closed")
			return
		case <-ctx.Done():
			conn.CloseWithError("yomo: parent context canceled")
			return
		case f := <-c.writeFrameChan:
			if err := conn.WriteFrame(f); err != nil {
				c.handleFrameError(err, reconnection)
			}
		case <-reconnection:
		reconnect:
			conn, err = c.connect(ctx, c.zipperAddr)
			if err != nil {
				if errors.As(err, new(ErrAuthenticateFailed)) {
					return
				}
				c.Logger.Error("reconnect to zipper error", "err", err)
				time.Sleep(time.Second)
				goto reconnect
			}
			go c.handleReadFrames(conn, reconnection)
		}
	}
}

// Connect connect client to server.
func (c *Client) Connect(ctx context.Context) error {
connect:
	fconn, err := c.connect(ctx, c.zipperAddr)
	if err != nil {
		if c.opts.connectUntilSucceed {
			c.Logger.Error("failed to connect to zipper, trying to reconnect", "err", err)
			time.Sleep(time.Second)
			goto connect
		}
		c.Logger.Error("can not connect to zipper", "err", err)
		return err
	}
	c.Logger.Info("connected to zipper")

	go c.runBackground(ctx, fconn)

	return nil
}

// WriteFrame write frame to client.
func (c *Client) WriteFrame(f frame.Frame) error {
	if c.opts.nonBlockWrite {
		return c.nonBlockWriteFrame(f)
	}
	return c.blockWriteFrame(f)
}

// blockWriteFrame writes frames in block mode, guaranteeing that frames are not lost.
func (c *Client) blockWriteFrame(f frame.Frame) error {
	select {
	case <-c.ctx.Done():
		return c.ctx.Err()
	case c.writeFrameChan <- f:
	}
	return nil
}

// nonBlockWriteFrame writes frames in non-blocking mode, without guaranteeing that frames will not be lost.
func (c *Client) nonBlockWriteFrame(f frame.Frame) error {
	select {
	case <-c.ctx.Done():
		return c.ctx.Err()
	case c.writeFrameChan <- f:
		return nil
	default:
		err := errors.New("yomo: client has lost connection")
		c.Logger.Debug("failed to write frame", "frame_type", f.Type().String(), "error", err)
		return err
	}
}

// Close close the client.
func (c *Client) Close() error {
	// break runBackgroud() for-loop.
	c.ctxCancel(fmt.Errorf("%s: local shutdown", c.clientType.String()))

	return nil
}

// handleFrameError handles errors that occur during frame reading and writing by performing the following actions:
// Sending the error to the error function (errorfn).
// Closing the client if the connecion has been closed.
// Always attempting to reconnect if an error is encountered.
func (c *Client) handleFrameError(err error, reconnection chan<- struct{}) {
	if err == nil {
		return
	}

	c.errorfn(err)

	// exit client program if stream has be closed.
	if se := new(yquic.ErrConnClosed); errors.As(err, &se) {
		c.ctxCancel(fmt.Errorf("%s: shutdown with error=%s", c.clientType.String(), se.Error()))
		return
	}

	// always attempting to reconnect if an error is encountered,
	// the error is mostly network error.
	select {
	case reconnection <- struct{}{}:
	default:
	}
}

// Wait waits client returning.
func (c *Client) Wait() {
	<-c.ctx.Done()
}

func (c *Client) handleReadFrames(fconn frame.Conn, reconnection chan struct{}) {
	for {
		f, err := fconn.ReadFrame()
		if err != nil {
			c.handleFrameError(err, reconnection)
			return
		}
		func() {
			defer func() {
				if e := recover(); e != nil {
					const size = 64 << 10
					buf := make([]byte, size)
					buf = buf[:runtime.Stack(buf, false)]

					perr := fmt.Errorf("%v", e)
					c.Logger.Error("stream panic", "err", perr)
					c.errorfn(fmt.Errorf("yomo: stream panic: %v\n%s", perr, buf))
				}
			}()
			c.handleFrame(f)
		}()
	}
}

func (c *Client) handleFrame(f frame.Frame) {
	switch ff := f.(type) {
	case *frame.RejectedFrame:
		c.Logger.Error("rejected error", "err", ff.Message)
		_ = c.Close()
	case *frame.DataFrame:
		c.processor(ff)
	case *frame.BackflowFrame:
		c.receiver(ff)
	default:
		c.Logger.Warn("received unexpected frame", "frame_type", f.Type().String())
	}
}

// SetDataFrameObserver sets the data frame handler.
func (c *Client) SetDataFrameObserver(fn func(*frame.DataFrame)) {
	c.processor = fn
}

// SetBackflowFrameObserver sets the backflow frame handler.
func (c *Client) SetBackflowFrameObserver(fn func(*frame.BackflowFrame)) {
	c.receiver = fn
}

// SetObserveDataTags set the data tag list that will be observed.
func (c *Client) SetObserveDataTags(tag ...frame.Tag) {
	c.opts.observeDataTags = tag
}

// SetErrorHandler set error handler
func (c *Client) SetErrorHandler(fn func(err error)) {
	c.errorfn = fn
	c.Logger.Debug("the error handler has been set")
}

// ClientID returns the ID of client.
func (c *Client) ClientID() string { return c.clientID }

// Name returns the name of client.
func (c *Client) Name() string { return c.name }

// Downstream represents a frame writer that can connect to an addr.
type Downstream interface {
	frame.Writer
	ID() string
	LocalName() string
	RemoteName() string
	Close() error
	Connect(context.Context) error
}

// TracerProvider returns the tracer provider of client.
func (c *Client) TracerProvider() oteltrace.TracerProvider {
	if c.tracerProvider == nil {
		return nil
	}
	if reflect.ValueOf(c.tracerProvider).IsNil() {
		return nil
	}
	return c.tracerProvider
}

// ErrAuthenticateFailed be returned when client control stream authenticate failed.
type ErrAuthenticateFailed struct {
	ReasonFromServer string
}

// Error returns a string that represents the ErrAuthenticateFailed error for the implementation of the error interface.
func (e ErrAuthenticateFailed) Error() string { return e.ReasonFromServer }
