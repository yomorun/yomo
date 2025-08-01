// Package core provides the core functions of YoMo.
package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/invopop/jsonschema"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/pkg/frame-codec/y3codec"
	"github.com/yomorun/yomo/pkg/id"
	yquic "github.com/yomorun/yomo/pkg/listener/quic"
	"github.com/yomorun/yomo/pkg/log"
)

// Client is the abstraction of a YoMo-Client. a YoMo-Client can be
// Source, Upstream Zipper or StreamFunction.
type Client struct {
	zipperAddr    string
	name          string                 // name of the client
	clientID      string                 // id of the client
	reconnCounter uint                   // counter for reconnection
	clientType    ClientType             // type of the client
	processor     func(*frame.DataFrame) // function to invoke when data arrived
	errorfn       func(error)            // function to invoke when error occured
	wantedTarget  string
	opts          *clientOptions
	Logger        *slog.Logger

	// ctx and ctxCancel manage the lifecycle of client.
	ctx       context.Context
	ctxCancel context.CancelCauseFunc

	// Receiving from the done channel to guarantee the connection is closed.
	done chan struct{}
	// Receiving from the reConnect channel to guarantee the client has stoped to reconnect during the reconnection.
	reConnect chan struct{}

	wrCh chan frame.Frame
	rdCh chan readOut
}

type readOut struct {
	err   error
	frame frame.Frame
}

// NewClient creates a new YoMo-Client.
func NewClient(appName, zipperAddr string, clientType ClientType, opts ...ClientOption) *Client {
	option := defaultClientOption()

	for _, o := range opts {
		o(option)
	}

	if qlogTraceEnabled() && option.quicConfig != nil {
		option.quicConfig.Tracer = qlogTracer
	}

	clientID := id.New()

	logger := option.logger

	ctx, ctxCancel := context.WithCancelCause(context.Background())

	return &Client{
		zipperAddr: zipperAddr,
		name:       appName,
		clientID:   clientID,
		processor:  func(df *frame.DataFrame) { logger.Warn("the processor has not been set") },
		clientType: clientType,
		opts:       option,
		Logger:     logger,
		ctx:        ctx,
		ctxCancel:  ctxCancel,

		done:      make(chan struct{}),
		reConnect: make(chan struct{}),
		wrCh:      make(chan frame.Frame),
		rdCh:      make(chan readOut),
	}
}

// SetWantedTarget set the wanted target string.
func (c *Client) SetWantedTarget(target string) {
	c.wantedTarget = target
}

// Connect connect client to server.
func (c *Client) Connect(ctx context.Context) error {
CONNECT:
	fconn, err := c.connect(ctx, c.zipperAddr)
	reconnect, err := c.handleConnectResult(err, c.opts.reconnect)
	if err != nil {
		return err
	}
	if reconnect {
		goto CONNECT
	}
	go c.runBackground(fconn)

	return nil
}

func (c *Client) handleConnectResult(err error, alwaysReconnect bool) (reconnect bool, se error) {
	if c.errorfn != nil && err != nil {
		c.errorfn(err)
	}
	select {
	case <-c.ctx.Done():
		close(c.reConnect)
		return false, err
	default:
	}
	if err == nil {
		c.Logger.Info("connected to zipper")
		log.InfoStatusEvent(os.Stdout, "[%s] is waiting for request...", c.name)
		return false, nil
	}
	if e := new(ErrRejected); errors.As(err, &e) {
		close(c.reConnect)
		c.Logger.Info("handshake be rejected", "err", e.Message)
		return false, err
	}
	if e := new(ErrConnectTo); errors.As(err, &e) {
		c.zipperAddr = e.Endpoint
		c.Logger.Info("connect to new endpoint", "endpoint", e.Endpoint)
		return true, nil
	}
	if alwaysReconnect {
		c.Logger.Error("failed to connect to zipper, trying to reconnect", "err", err)
		time.Sleep(time.Second)
		return true, nil
	}
	c.Logger.Error("cannot connect to zipper", "err", err)
	return false, err
}

func (c *Client) runBackground(conn frame.Conn) {
	if closed := c.handleConn(conn); closed {
		return
	}

	// try reconnect to zipper.
	var err error
	for {
		conn, err = c.connect(c.ctx, c.zipperAddr)
		reconnect, err := c.handleConnectResult(err, true)
		if err != nil {
			return
		}
		if reconnect {
			time.Sleep(time.Second)
			continue
		}
		if closed := c.handleConn(conn); closed {
			return
		}
	}
}

func (c *Client) handleConn(conn frame.Conn) (closed bool) {
	if err := c.serveConn(conn); err != nil {
		if c.errorfn != nil {
			c.errorfn(err)
		} else {
			c.Logger.Error("handle frame failed", "err", err)
		}
		// Exit client program if the connection has be closed.
		if se := new(frame.ErrConnClosed); errors.As(err, &se) {
			if se.Remote {
				c.ctxCancel(fmt.Errorf("%s: shutdown with error=%s", c.clientType.String(), se.ErrorMessage))
			}
			return true
		}
	}
	return false
}

func (c *Client) connect(ctx context.Context, addr string) (frame.Conn, error) {
	conn, err := yquic.DialAddr(ctx, addr, y3codec.Codec(), y3codec.PacketReadWriter(), c.opts.tlsConfig, c.opts.quicConfig)
	if err != nil {
		return conn, err
	}

	// refresh client id in order to avoid id conflicts on the server-side
	clientID := fmt.Sprintf("%s-%d", c.clientID, c.reconnCounter)
	c.reconnCounter++

	hf := &frame.HandshakeFrame{
		Name:            c.name,
		ID:              clientID,
		ClientType:      byte(c.clientType),
		ObserveDataTags: c.opts.observeDataTags,
		AuthName:        c.opts.credential.Name(),
		AuthPayload:     c.opts.credential.Payload(),
		Version:         Version,
		WantedTarget:    c.wantedTarget,
	}

	jsonschemaBytes, err := c.handshakeWithDefinition()
	if err != nil {
		return nil, err
	}
	hf.FunctionDefinition = jsonschemaBytes

	// aiFunctionDefinition has a higher priority than description with input model.
	if c.opts.aiFunctionDefinition != "" {
		hf.FunctionDefinition = []byte(c.opts.aiFunctionDefinition)
	}

	if err := conn.WriteFrame(hf); err != nil {
		return conn, err
	}

	received, err := conn.ReadFrame()
	if err != nil {
		return nil, err
	}

	switch received.Type() {
	case frame.TypeHandshakeAckFrame:
		return conn, nil
	case frame.TypeRejectedFrame:
		err := &ErrRejected{Message: received.(*frame.RejectedFrame).Message}
		_ = conn.CloseWithError(err.Error())
		return nil, err
	case frame.TypeConnectToFrame:
		ff := received.(*frame.ConnectToFrame)
		err := &ErrConnectTo{Endpoint: ff.Endpoint}
		_ = conn.CloseWithError(err.Error())
		return nil, err
	}
	// other frame type
	err = &ErrRejected{
		Message: fmt.Sprintf("handshake failed: read unexpected frame, frame read: %s", received.Type().String()),
	}
	_ = conn.CloseWithError(err.Error())
	return nil, err
}

func (c *Client) handshakeWithDefinition() ([]byte, error) {
	if c.clientType != ClientTypeStreamFunction {
		return nil, nil
	}
	// register ai function definition
	functionDefinition, err := ParseAIFunctionDefinition(c.name, c.opts.aiFunctionDescription, c.opts.aiFunctionInputModel)
	if err != nil {
		c.Logger.Error("parse ai function definition error", "err", err)
		return nil, err
	}
	// ai function definition is not be found
	if functionDefinition == nil {
		return nil, nil
	}
	log.InfoStatusEvent(os.Stdout, "Function Calling jsonschema: %s", string(functionDefinition))
	return functionDefinition, nil
}

// ParseAIFunctionDefinition generates a function definition in jsonschema based on the function name, description, and input model.
func ParseAIFunctionDefinition(sfnName, aiFunctionDescription string, aiFunctionInputModel any) ([]byte, error) {
	if aiFunctionDescription == "" {
		return nil, nil
	}
	// parse ai function definition
	function := &ai.FunctionDefinition{
		Name:        sfnName,
		Description: aiFunctionDescription,
	}
	if aiFunctionInputModel != nil {
		functionParameters, err := parseAIFunctionParameters(aiFunctionInputModel)
		if err != nil {
			return nil, fmt.Errorf("parse function parameters error: %s", err.Error())
		}
		function.Parameters = functionParameters
	}
	buf, err := json.Marshal(function)
	if err != nil {
		return nil, fmt.Errorf("marshal function definition error: %s", err.Error())
	}
	return buf, nil
}

func parseAIFunctionParameters(inputModel any) (*ai.FunctionParameters, error) {
	schema := jsonschema.Reflect(inputModel)
	for _, m := range schema.Definitions {
		functionParameters := &ai.FunctionParameters{
			Type:       m.Type,
			Required:   m.Required,
			Properties: make(map[string]*ai.ParameterProperty),
		}

		for pair := m.Properties.Oldest(); pair != nil; pair = pair.Next() {
			functionParameters.Properties[pair.Key] = &ai.ParameterProperty{
				Type:        pair.Value.Type,
				Description: pair.Value.Description,
				Enum:        pair.Value.Enum,
			}
		}
		return functionParameters, nil
	}
	return nil, errors.New("invalid function definition")
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
		return context.Cause(c.ctx)
	case c.wrCh <- f:
	}
	return nil
}

// nonBlockWriteFrame writes frames in non-blocking mode, without guaranteeing that frames will not be lost.
func (c *Client) nonBlockWriteFrame(f frame.Frame) error {
	select {
	case <-c.ctx.Done():
		return context.Cause(c.ctx)
	case c.wrCh <- f:
		return nil
	case <-time.After(time.Second):
		return errors.New("yomo: non-block write frame timeout")
	}
}

// Close close the client.
func (c *Client) Close() error {
	// break runBackgroud() for-loop.
	c.ctxCancel(fmt.Errorf("%s: shutdown", c.clientType.String()))

	select {
	case <-c.done:
	case <-c.reConnect:
	}

	return nil
}

// Wait waits client returning.
func (c *Client) Wait() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)

	select {
	case s := <-ch:
		c.ctxCancel(fmt.Errorf("%s: shutdown with signal=%s", c.clientType.String(), s))
	case <-c.done:
	case <-c.reConnect:
	}
}

func (c *Client) serveConn(conn frame.Conn) error {
	go func() {
		for {
			f, err := conn.ReadFrame()
			if err != nil {
				c.rdCh <- readOut{err: err}
				return
			}
			c.rdCh <- readOut{frame: f}
		}
	}()

	for {
		select {
		case <-c.ctx.Done():
			err := context.Cause(c.ctx)
			conn.CloseWithError(err.Error())
			close(c.done)
			return err
		case f := <-c.wrCh:
			if err := conn.WriteFrame(f); err != nil {
				return err
			}
		case out := <-c.rdCh:
			if err := out.err; err != nil {
				return err
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
				c.handleFrame(out.frame)
			}()
		}
	}
}

func (c *Client) handleFrame(f frame.Frame) {
	switch ff := f.(type) {
	case *frame.GoawayFrame:
		c.Logger.Error("goaway error", "err", ff.Message)
		_ = c.Close()
	case *frame.RejectedFrame:
		c.Logger.Error("rejected error", "err", ff.Message)
		_ = c.Close()
	case *frame.DataFrame:
		c.processor(ff)
	default:
		c.Logger.Warn("received unexpected frame", "frame_type", f.Type().String())
	}
}

// SetDataFrameObserver sets the data frame handler.
func (c *Client) SetDataFrameObserver(fn func(*frame.DataFrame)) {
	c.processor = fn
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

// DisableOtelTrace return if disable otel trace.
func (c *Client) DisableOtelTrace() bool { return c.opts.disableOtelTrace }

// Downstream represents a frame writer that can connect to an addr.
type Downstream interface {
	frame.Writer
	ID() string
	LocalName() string
	RemoteName() string
	Close() error
	Connect(context.Context) error
}
