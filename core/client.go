// Package core provides the core functions of YoMo.
package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"runtime"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/pkg/frame-codec/y3codec"
	"github.com/yomorun/yomo/pkg/id"
	oteltrace "go.opentelemetry.io/otel/trace"
	"golang.org/x/exp/slog"
)

// Client is the abstraction of a YoMo-Client. a YoMo-Client can be
// Source, Upstream Zipper or StreamFunction.
type Client struct {
	name           string                     // name of the client
	clientID       string                     // id of the client
	streamType     StreamType                 // type of the dataStream
	processor      func(*frame.DataFrame)     // function to invoke when data arrived
	receiver       func(*frame.BackflowFrame) // function to invoke when data is processed
	errorfn        func(error)                // function to invoke when error occured
	opts           *clientOptions
	logger         *slog.Logger
	tracerProvider oteltrace.TracerProvider

	// ctx and ctxCancel manage the lifecycle of client.
	ctx       context.Context
	ctxCancel context.CancelCauseFunc

	writeFrameChan chan frame.Frame
}

// NewClient creates a new YoMo-Client.
func NewClient(appName string, connType ClientType, opts ...ClientOption) *Client {
	option := defaultClientOption()

	for _, o := range opts {
		o(option)
	}
	clientID := id.New()

	logger := option.logger.With("component", connType.String(), "client_id", clientID, "client_name", appName)

	if option.credential != nil {
		logger.Info("use credential", "credential_name", option.credential.Name())
	}

	ctx, ctxCancel := context.WithCancelCause(context.Background())

	return &Client{
		name:           appName,
		clientID:       clientID,
		processor:      func(df *frame.DataFrame) { logger.Warn("the processor has not been set") },
		receiver:       func(bf *frame.BackflowFrame) { logger.Warn("the receiver has not been set") },
		streamType:     connType,
		opts:           option,
		logger:         logger,
		tracerProvider: option.tracerProvider,
		errorfn:        func(err error) { logger.Error("client err", "err", err) },
		writeFrameChan: make(chan frame.Frame),
		ctx:            ctx,
		ctxCancel:      ctxCancel,
	}
}

type connectResult struct {
	conn quic.Connection
	fs   *FrameStream
	err  error
}

func newConnectResult(conn quic.Connection, fs *FrameStream, err error) *connectResult {
	return &connectResult{
		conn: conn,
		fs:   fs,
		err:  err,
	}
}

func (c *Client) connect(ctx context.Context, addr string) *connectResult {
	conn, err := quic.DialAddr(ctx, addr, c.opts.tlsConfig, c.opts.quicConfig)
	if err != nil {
		return newConnectResult(conn, nil, err)
	}

	stream, err := conn.OpenStream()
	if err != nil {
		return newConnectResult(conn, nil, err)
	}

	fs := NewFrameStream(stream, y3codec.Codec(), y3codec.PacketReadWriter())

	hf := &frame.HandshakeFrame{
		Name:            c.name,
		ID:              c.clientID,
		StreamType:      byte(c.streamType),
		ObserveDataTags: c.opts.observeDataTags,
		AuthName:        c.opts.credential.Name(),
		AuthPayload:     c.opts.credential.Payload(),
	}

	if err := fs.WriteFrame(hf); err != nil {
		return newConnectResult(conn, nil, err)
	}

	received, err := fs.ReadFrame()
	if err != nil {
		return newConnectResult(conn, nil, err)
	}

	switch received.Type() {
	case frame.TypeRejectedFrame:
		se := ErrAuthenticateFailed{received.(*frame.RejectedFrame).Message}
		return newConnectResult(conn, fs, se)
	case frame.TypeHandshakeAckFrame:
		return newConnectResult(conn, fs, nil)
	default:
		se := ErrAuthenticateFailed{
			fmt.Sprintf("authentication failed: read unexcepted frame, frame read: %s", received.Type().String()),
		}
		return newConnectResult(conn, fs, se)
	}
}

func (c *Client) runBackground(ctx context.Context, addr string, conn quic.Connection, fs *FrameStream) {
	reconnection := make(chan struct{})

	go c.handleReadFrames(fs, reconnection)

	for {
		select {
		case <-c.ctx.Done():
			fs.underlying.Close()
			return
		case <-ctx.Done():
			fs.underlying.Close()
			return
		case f := <-c.writeFrameChan:
			if err := fs.WriteFrame(f); err != nil {
				c.handleFrameError(err, reconnection)
			}
		case <-reconnection:
		reconnect:
			cr := c.connect(ctx, addr)
			if err := cr.err; err != nil {
				if errors.As(err, new(ErrAuthenticateFailed)) {
					return
				}
				c.logger.Error("reconnect to zipper error", "err", cr.err)
				time.Sleep(time.Second)
				goto reconnect
			}
			go c.handleReadFrames(fs, reconnection)
		}
	}
}

// Connect connect client to server.
func (c *Client) Connect(ctx context.Context, addr string) error {
	if c.streamType == StreamTypeStreamFunction && len(c.opts.observeDataTags) == 0 {
		return errors.New("yomo: streamFunction cannot observe data because the required tag has not been set")
	}

	c.logger = c.logger.With("zipper_addr", addr)

connect:
	result := c.connect(ctx, addr)
	if result.err != nil {
		if c.opts.connectUntilSucceed {
			c.logger.Error("failed to connect to zipper, trying to reconnect", "err", result.err)
			time.Sleep(time.Second)
			goto connect
		}
		c.logger.Error("can not connect to zipper", "err", result.err)
		return result.err
	}
	c.logger.Info("connected to zipper")

	go c.runBackground(ctx, addr, result.conn, result.fs)

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
		c.logger.Debug("failed to write frame", "frame_type", f.Type().String(), "error", err)
		return err
	}
}

// Close close the client.
func (c *Client) Close() error {
	// break runBackgroud() for-loop.
	c.ctxCancel(fmt.Errorf("%s: local shutdown", c.streamType.String()))

	return nil
}

// handleFrameError handles errors that occur during frame reading and writing by performing the following actions:
// Sending the error to the error function (errorfn).
// Closing the client if the data stream has been closed.
// Always attempting to reconnect if an error is encountered.
func (c *Client) handleFrameError(err error, reconnection chan<- struct{}) {
	if err == nil {
		return
	}

	c.errorfn(err)

	// exit client program if stream has be closed.
	if err == io.EOF {
		c.ctxCancel(fmt.Errorf("%s: remote shutdown", c.streamType.String()))
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

func (c *Client) handleReadFrames(fs *FrameStream, reconnection chan struct{}) {
	for {
		f, err := fs.ReadFrame()
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
					c.logger.Error("stream panic", "err", perr)
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
		c.logger.Error("rejected error", "err", ff.Message)
		_ = c.Close()
	case *frame.DataFrame:
		c.processor(ff)
	case *frame.BackflowFrame:
		c.receiver(ff)
	default:
		c.logger.Warn("data stream received unexpected frame", "frame_type", f.Type().String())
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

// Logger get client's logger instance, you can customize this using `yomo.WithLogger`
func (c *Client) Logger() *slog.Logger {
	return c.logger
}

// SetErrorHandler set error handler
func (c *Client) SetErrorHandler(fn func(err error)) {
	c.errorfn = fn
	c.logger.Debug("the error handler has been set")
}

// ClientID returns the ID of client.
func (c *Client) ClientID() string { return c.clientID }

// Name returns the name of client.
func (c *Client) Name() string { return c.name }

// FrameWriterConnection represents a frame writer that can connect to an addr.
type FrameWriterConnection interface {
	frame.Writer
	Name() string
	Close() error
	Connect(context.Context, string) error
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
