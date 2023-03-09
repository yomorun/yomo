// Package core provides the core functions of YoMo.
package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/yerr"
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
	conn       quic.Connection            // quic connection
	fs         frame.ReadWriter           // yomo abstract stream
	state      ConnState                  // state of the connection
	processor  func(*frame.DataFrame)     // function to invoke when data arrived
	receiver   func(*frame.BackflowFrame) // function to invoke when data is processed
	errorfn    func(error)                // function to invoke when error occured
	closefn    func()                     // function to invoke when client closed
	addr       string                     // the address of server connected to
	mu         sync.Mutex
	opts       *clientOptions
	localAddr  string // client local addr, it will be changed on reconnect
	logger     *slog.Logger
	errc       chan error
}

// NewClient creates a new YoMo-Client.
func NewClient(appName string, connType StreamType, opts ...ClientOption) *Client {
	option := defaultClientOption()

	for _, o := range opts {
		o(option)
	}
	clientID := id.New()

	logger := option.logger.With("component", "client", "client_type", connType.String(), "client_id", clientID, "client_name", appName)

	if option.credential != nil {
		logger.Info("use credential", "credential_name", option.credential.Name())
	}

	return &Client{
		name:       appName,
		clientID:   clientID,
		streamType: connType,
		state:      ConnStateReady,
		opts:       option,
		errc:       make(chan error),
		logger:     logger,
	}
}

// Connect connects to YoMo-Zipper.
func (c *Client) Connect(ctx context.Context, addr string) error {
	// TODO: refactor this later as a Connection Manager
	// reconnect
	// for download zipper
	// If you do not check for errors, the connection will be automatically reconnected
	go c.reconnect(ctx, addr)

	// connect
	return c.connect(ctx, addr)
}

func (c *Client) connect(ctx context.Context, addr string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state != ConnStateReady && c.state != ConnStateDisconnected {
		return nil
	}

	c.addr = addr
	c.state = ConnStateConnecting

	// create quic connection
	conn, err := quic.DialAddrContext(ctx, addr, c.opts.tlsConfig, c.opts.quicConfig)
	if err != nil {
		c.state = ConnStateDisconnected
		return err
	}
	c.conn = conn

	// quic stream
	stream0, err := conn.OpenStreamSync(ctx)
	if err != nil {
		c.state = ConnStateDisconnected
		return err
	}

	controlStream := NewFrameStream(stream0)

	// send authentication
	authentication := frame.NewAuthenticationFrame(
		c.opts.credential.Name(),
		c.opts.credential.Payload(),
	)
	if err := controlStream.WriteFrame(authentication); err != nil {
		c.state = ConnStateDisconnected
		return err
	}
	c.logger.Debug("AuthenticationFrame be Writed")

	if err := c.authenticationAck(controlStream); err != nil {
		c.state = ConnStateDisconnected
		return err
	}
	c.logger.Debug("Receive handshakeAckFrame")

	if err := c.acquireDataStream(controlStream); err != nil {
		c.state = ConnStateDisconnected
		return err
	}

	c.state = ConnStateConnected
	c.localAddr = c.conn.LocalAddr().String()

	datsStream, err := conn.AcceptStream(ctx)
	if err != nil {
		c.logger.Error("Failed to accept data stream", err)
		return err
	}
	c.logger.Debug("DataStream ready")

	c.fs = NewFrameStream(datsStream)

	// receiving frames
	go func() {
		closeConn, closeClient, err := c.handleFrame()
		c.logger.Debug("connected to YoMo-Zipper", "close_conn", closeConn, "close_client", closeClient, "error", err)

		c.mu.Lock()
		defer c.mu.Unlock()

		if c.state == ConnStateClosed {
			return
		}

		c.state = ConnStateDisconnected
		c.errc <- err

		datsStream.Close()
		if closeConn {
			code := yerr.ErrorCodeClientAbort
			if e, ok := err.(yerr.YomoError); ok {
				code = e.ErrorCode()
			}
			c.conn.CloseWithError(code.To(), err.Error())
		}

		if closeClient {
			c.close()
		}
	}()

	return nil
}

// authenticationAck waits authentication ack, ack maybe ok or not ok.
func (c *Client) authenticationAck(controlStream frame.Reader) error {
	f, err := controlStream.ReadFrame()
	if err != nil {
		return err
	}

	ff, ok := f.(*frame.AuthenticationAckFrame)
	if !ok {
		return fmt.Errorf("yomo: read unexcept frame during waiting authentication ack, frame readed: %s", f.Type().String())
	}

	if !ff.OK() {
		return errors.New(ff.Reason())
	}

	return nil
}

func (c *Client) acquireDataStream(controlStream frame.ReadWriter) error {
	err := controlStream.WriteFrame(frame.NewHandshakeFrame(
		c.name,
		c.clientID,
		byte(c.streamType),
		c.opts.observeDataTags,
		[]byte{}, // The stream does not require metadata currently.
	))

	if err != nil {
		c.state = ConnStateDisconnected
	}

	return err
}

// handleFrame handles the logic when receiving frame from server.
// handleFrame returns if connection and client should be closed after handle frame,
// handleFrame returns two boolean, the first indicates whether to close connection (or client), second
// close stream, It's will reconnect if connection (or client) is closed,
// It's will exit program if client is closed. The Goaway logic is always close client.
func (c *Client) handleFrame() (bool, bool, error) {
	for {
		// this will block until a frame is received
		f, err := c.fs.ReadFrame()
		if err != nil {
			if err == io.EOF {
				return true, false, err
			} else if strings.HasPrefix(err.Error(), "unknown frame type") {
				c.logger.Warn("unknown frame type", "error", err)
				continue
			} else if e, ok := err.(*quic.IdleTimeoutError); ok {
				return false, false, e
			} else if e, ok := err.(*quic.ApplicationError); ok {
				return false, e.ErrorCode == yerr.ErrorCodeGoaway.To() || e.ErrorCode == yerr.ErrorCodeRejected.To(), e
			} else if errors.Is(err, net.ErrClosed) {
				return false, false, err
			}
			return true, false, err
		}

		// read frame
		// first, get frame type
		frameType := f.Type()
		c.logger.Debug("handleFrame", "frame_type", frameType)
		switch frameType {
		case frame.TagOfHandshakeAckFrame:
			continue
		case frame.TagOfRejectedFrame:
			if v, ok := f.(*frame.RejectedFrame); ok {
				return true, true, errors.New(v.Message())
			}
		case frame.TagOfGoawayFrame:
			if v, ok := f.(*frame.GoawayFrame); ok {
				return true, true, errors.New(v.Message())
			}
		case frame.TagOfCloseStreamFrame:
			if v, ok := f.(*frame.CloseStreamFrame); ok {
				return true, v.StreamID() == c.clientID, errors.New(v.Reason())
			}
		case frame.TagOfDataFrame: // DataFrame carries user's data
			if v, ok := f.(*frame.DataFrame); ok {
				if c.processor == nil {
					c.logger.Warn("processor is nil")
				} else {
					c.processor(v)
				}
				v.Clean()
			}
		case frame.TagOfBackflowFrame:
			if v, ok := f.(*frame.BackflowFrame); ok {
				if c.receiver == nil {
					c.logger.Warn("receiver is nil")
				} else {
					c.receiver(v)
				}
			}
		default:
			c.logger.Warn("unknown or unsupported frame", "frame_type", frameType)
		}
	}
}

// Close the client.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state == ConnStateClosed {
		return nil
	}

	if c.conn != nil {
		c.conn.CloseWithError(yerr.ErrorCodeClientAbort.To(), "client ask to close")
	}

	return c.close()
}

func (c *Client) close() error {
	c.logger.Info("close the connection")

	// close error channel so that close handler function will be called
	close(c.errc)

	c.state = ConnStateClosed
	return nil
}

// WriteFrame writes a frame to the connection, gurantee threadsafe.
func (c *Client) WriteFrame(frm frame.Frame) error {
	if c.state != ConnStateConnected {
		return errors.New("client connection isn't connected")
	}

	if err := c.fs.WriteFrame(frm); err != nil {
		return err
	}

	return nil
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

// reconnect the connection between client and server.
func (c *Client) reconnect(ctx context.Context, addr string) {
	t := time.NewTicker(1 * time.Second)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			c.logger.Debug("context.Done", "error", ctx.Err())
			return
		case err, ok := <-c.errc:
			if c.errorfn != nil && err != nil {
				c.errorfn(err)
			}
			if !ok && c.closefn != nil {
				c.closefn()
				return
			}
		case <-t.C:
			c.mu.Lock()
			state := c.state
			c.mu.Unlock()
			if state == ConnStateDisconnected {
				c.logger.Info("reconnecting to YoMo-Zipper")
				err := c.connect(ctx, addr)
				if err != nil {
					c.logger.Error("reconnecting to YoMo-Zipper", err)
				}
			}
		}
	}
}

// RemoteAddr returns the remote address of the client connected to.
func (c *Client) RemoteAddr() string { return c.addr }

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

// SetCloseHandler set close handler
func (c *Client) SetCloseHandler(fn func()) {
	c.closefn = fn
}

// ClientID return the client ID
func (c *Client) ClientID() string {
	return c.clientID
}

// State return the state of client,
// NewClient returned, state is `Ready`, after calling `Connect()`,
// the state is `Connected` if success is returned otherwise it is `Disconnected`.
func (c *Client) State() ConnState {
	c.mu.Lock()
	state := c.state
	c.mu.Unlock()

	return state
}

// String returns client's name and addr format as a string.
func (c *Client) String() string { return fmt.Sprintf("name:%s, addr: %s", c.name, c.RemoteAddr()) }
