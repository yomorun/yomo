// Package core provides the core functions of YoMo.
package core

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/pkg/id"
	"golang.org/x/exp/slog"
)

// ClientOption YoMo client options
type ClientOption func(*clientOptions)

// Client is the abstraction of a YoMo-Client. a YoMo-Client can be
// Source, Upstream Zipper or StreamFunction.
type Client struct {
	name       string                     // name of the client
	clientID   string                     // id of the client
	streamType StreamType                 // type of the dataStream
	processor  func(*frame.DataFrame)     // function to invoke when data arrived
	receiver   func(*frame.BackflowFrame) // function to invoke when data is processed
	errorfn    func(error)                // function to invoke when error occured
	opts       *clientOptions
	logger     *slog.Logger

	// ctx and ctxCancel manage the lifecycle of client.
	ctx       context.Context
	ctxCancel context.CancelFunc

	writeFrameChan chan frame.Frame
}

// NewClient creates a new YoMo-Client.
func NewClient(appName string, connType ClientType, opts ...ClientOption) *Client {
	option := defaultClientOption()

	for _, o := range opts {
		o(option)
	}
	clientID := id.New()

	logger := option.logger.With("component", "client", "client_type", connType.String(), "client_id", clientID, "client_name", appName)

	if option.credential != nil {
		logger.Info("use credential", "credential_name", option.credential.Name())
	}

	ctx, ctxCancel := context.WithCancel(context.Background())

	return &Client{
		name:           appName,
		clientID:       clientID,
		streamType:     connType,
		opts:           option,
		logger:         logger,
		errorfn:        func(err error) { logger.Error("client err", err) },
		writeFrameChan: make(chan frame.Frame),
		ctx:            ctx,
		ctxCancel:      ctxCancel,
	}
}

// Connect connect client to server.
func (c *Client) Connect(ctx context.Context, addr string) error {
	controlStream, dataStream, err := c.openStream(ctx, addr)
	if err != nil {
		c.logger.Error("connect error", err)
		return err
	}

	go c.runBackground(ctx, addr, controlStream, dataStream)

	return nil
}

func (c *Client) runBackground(ctx context.Context, addr string, controlStream ClientControlStream, dataStream DataStream) {
	reconnection := make(chan struct{})

	go c.processStream(controlStream, dataStream, reconnection)

	for {
		select {
		case <-c.ctx.Done():
			c.cleanStream(controlStream, nil)
			return
		case <-ctx.Done():
			c.cleanStream(controlStream, ctx.Err())
			return
		case <-reconnection:
		RECONNECT:
			var err error
			controlStream, dataStream, err = c.openStream(ctx, addr)
			if err != nil {
				if errors.As(err, new(ErrAuthenticateFailed)) {
					c.cleanStream(controlStream, err)
					return
				}
				c.logger.Error("client reconnect error", err)
				time.Sleep(time.Second)
				goto RECONNECT
			}
			go c.processStream(controlStream, dataStream, reconnection)
		}
	}
}

// WriteFrame write frame to client.
func (c *Client) WriteFrame(f frame.Frame) error {
	c.writeFrameChan <- f
	return nil
}

func (c *Client) cleanStream(controlStream ClientControlStream, err error) {
	errString := ""
	if err != nil {
		errString = err.Error()
		c.logger.Error("client cancel with error", err)
	}

	// controlStream is nil represents that client is not connected.
	if controlStream == nil {
		return
	}

	controlStream.CloseWithError(0, errString)
}

// Close close the client.
func (c *Client) Close() error {
	// break runBackgroud() for-loop.
	c.ctxCancel()

	return nil
}

func (c *Client) openControlStream(ctx context.Context, addr string) (ClientControlStream, error) {
	controlStream, err := OpenClientControlStream(ctx, addr, c.opts.tlsConfig, c.opts.quicConfig, metadata.DefaultDecoder(), c.logger)
	if err != nil {
		return controlStream, err
	}

	if err := controlStream.Authenticate(c.opts.credential); err != nil {
		return controlStream, err
	}

	return controlStream, nil
}

func (c *Client) openStream(ctx context.Context, addr string) (ClientControlStream, DataStream, error) {
	controlStream, err := c.openControlStream(ctx, addr)
	if err != nil {
		return controlStream, nil, err
	}
	dataStream, err := c.openDataStream(ctx, controlStream)
	if err != nil {
		return controlStream, dataStream, err
	}

	return controlStream, dataStream, nil
}

func (c *Client) openDataStream(ctx context.Context, controlStream ClientControlStream) (DataStream, error) {
	handshakeFrame := frame.NewHandshakeFrame(
		c.name,
		c.clientID,
		byte(c.streamType),
		c.opts.observeDataTags,
		[]byte{}, // The stream does not require metadata currently.
	)

	err := controlStream.RequestStream(handshakeFrame)
	if err != nil {
		return nil, err
	}

	return controlStream.AcceptStream(ctx)
}

func (c *Client) processStream(controlStream ClientControlStream, dataStream DataStream, reconnection chan<- struct{}) {
	defer dataStream.Close()

	readFrameChan := c.readFrame(dataStream)

	for {
		select {
		case result := <-readFrameChan:
			if err := result.err; err != nil {
				c.handleFrameError(err, reconnection)
				return
			}
			c.handleFrame(result.frame)
		case f := <-c.writeFrameChan:
			err := dataStream.WriteFrame(f)
			// restore DataFrame.
			if d, ok := f.(*frame.DataFrame); ok {
				d.Clean()
			}
			c.handleFrameError(err, reconnection)
		}
	}
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
		c.ctxCancel()
		return
	}

	// always attempting to reconnect if an error is encountered,
	// the error is mostly network error.
	reconnection <- struct{}{}
}

// Wait waits client error returning.
func (c *Client) Wait() {
	<-c.ctx.Done()
}

type readResult struct {
	frame frame.Frame
	err   error
}

func (c *Client) readFrame(dataStream DataStream) chan readResult {
	readChan := make(chan readResult)
	go func() {
		for {
			f, err := dataStream.ReadFrame()
			readChan <- readResult{f, err}
			if err != nil {
				return
			}
		}
	}()

	return readChan
}

func (c *Client) handleFrame(f frame.Frame) {
	switch ff := f.(type) {
	case *frame.DataFrame:
		if c.processor == nil {
			c.logger.Warn("client processor has not been set")
		} else {
			c.processor(ff)
		}
	case *frame.BackflowFrame:
		if c.receiver == nil {
			c.logger.Warn("client receiver has not been set")
		} else {
			c.receiver(ff)
		}
	default:
		c.logger.Warn("client data stream receive unexcepted frame", "frame_type", f)
	}
}

// SetDataFrameObserver sets the data frame handler.
func (c *Client) SetDataFrameObserver(fn func(*frame.DataFrame)) {
	c.processor = fn
	c.logger.Debug("SetDataFrameObserver")
}

// SetBackflowFrameObserver sets the backflow frame handler.
func (c *Client) SetBackflowFrameObserver(fn func(*frame.BackflowFrame)) {
	c.receiver = fn
	c.logger.Debug("SetBackflowFrameObserver")
}

// SetObserveDataTags set the data tag list that will be observed.
// Deprecated: use yomo.WithObserveDataTags instead
func (c *Client) SetObserveDataTags(tag ...frame.Tag) {
	c.opts.observeDataTags = append(c.opts.observeDataTags, tag...)
}

// Logger get client's logger instance, you can customize this using `yomo.WithLogger`
func (c *Client) Logger() *slog.Logger {
	return c.logger
}

// SetErrorHandler set error handler
func (c *Client) SetErrorHandler(fn func(err error)) {
	c.errorfn = fn
}

// ClientID return the client ID
func (c *Client) ClientID() string {
	return c.clientID
}
