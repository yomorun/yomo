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

	"github.com/lucas-clemente/quic-go"
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
	clientType ClientType                 // type of the connection
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
func NewClient(appName string, connType ClientType, opts ...ClientOption) *Client {
	option := defaultClientOption()

	for _, o := range opts {
		o(option)
	}
	clientID := id.New()

	logger := slog.With("component", "client", "type", connType.String(), "client_id", clientID, "client_name", appName)

	return &Client{
		name:       appName,
		clientID:   clientID,
		clientType: connType,
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
	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		c.state = ConnStateDisconnected
		return err
	}
	c.fs = NewFrameStream(stream)

	// send handshake
	handshake := frame.NewHandshakeFrame(
		c.name,
		c.clientID,
		byte(c.clientType),
		c.opts.observeDataTags,
		c.opts.credential.Name(),
		c.opts.credential.Payload(),
	)
	if err := c.fs.WriteFrame(handshake); err != nil {
		c.state = ConnStateDisconnected
		return err
	}

	if _, err := frame.ReadUntil(c.fs, frame.TagOfHandshakeAckFrame, 10*time.Second); err != nil {
		c.state = ConnStateDisconnected
		return err
	}

	c.state = ConnStateConnected
	c.localAddr = c.conn.LocalAddr().String()

	c.logger = slog.With("local_addr", c.localAddr, "reomote", c.RemoteAddr())

	c.logger.Debug("connected to YoMo-Zipper")

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

		stream.Close()
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

// handleFrame handles the logic when receiving frame from server.
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
		case frame.TagOfRejectedFrame:
			if v, ok := f.(*frame.RejectedFrame); ok {
				return true, true, errors.New(v.Message())
			}
		case frame.TagOfGoawayFrame:
			if v, ok := f.(*frame.GoawayFrame); ok {
				return true, true, errors.New(v.Message())
			}
		case frame.TagOfDataFrame: // DataFrame carries user's data
			if v, ok := f.(*frame.DataFrame); ok {
				if c.processor == nil {
					c.logger.Warn("processor is nil")
				} else {
					c.processor(v)
				}
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
			c.logger.Warn("unknown or unsupported frame", "frame_type", frameType.String())
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
	c.logger.Debug("close the connection")

	// close error channel so that close handler function will be called
	close(c.errc)

	c.state = ConnStateClosed
	return nil
}

// WriteFrame writes a frame to the connection, gurantee threadsafe.
func (c *Client) WriteFrame(frm frame.Frame) error {
	c.logger.Debug("close the connection", "client_state", c.State(), "frame_type", frm.Type().String())

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
	c.logger.Debug("SetDataFrameObserver", "processor", c.processor)
}

// SetBackflowFrameObserver sets the backflow frame handler.
func (c *Client) SetBackflowFrameObserver(fn func(*frame.BackflowFrame)) {
	c.receiver = fn
	c.logger.Debug("SetBackflowFrameObserver", "receiver", c.receiver)
}

// reconnect the connection between client and server.
func (c *Client) reconnect(ctx context.Context, addr string) {
	t := time.NewTicker(1 * time.Second)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			c.logger.Debug("context.Done", "receiver", "error", ctx.Err())
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
				c.logger.Debug("reconnecting to YoMo-Zipper")
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
