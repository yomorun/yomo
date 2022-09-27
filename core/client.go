package core

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/lucas-clemente/quic-go"
	"github.com/yomorun/yomo/core/auth"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/log"
	"github.com/yomorun/yomo/core/yerr"
	"github.com/yomorun/yomo/pkg/id"
	"github.com/yomorun/yomo/pkg/logger"
	pkgtls "github.com/yomorun/yomo/pkg/tls"
)

// ClientOption YoMo client options
type ClientOption func(*ClientOptions)

// Client is the abstraction of a YoMo-Client. a YoMo-Client can be
// Source, Upstream Zipper or StreamFunction.
type Client struct {
	name       string                     // name of the client
	clientID   string                     // id of the client
	clientType ClientType                 // type of the connection
	conn       quic.Connection            // quic connection
	fs         *FrameStream               // yomo abstract stream
	state      ConnState                  // state of the connection
	processor  func(*frame.DataFrame)     // function to invoke when data arrived
	receiver   func(*frame.BackflowFrame) // function to invoke when data is processed
	errorfn    func(error)                // function to invoke when error occured
	closefn    func()                     // function to invoke when client closed
	addr       string                     // the address of server connected to
	mu         sync.Mutex
	opts       ClientOptions
	localAddr  string // client local addr, it will be changed on reconnect
	logger     log.Logger
	errc       chan error
}

// NewClient creates a new YoMo-Client.
func NewClient(appName string, connType ClientType, opts ...ClientOption) *Client {
	c := &Client{
		name:       appName,
		clientID:   id.New(),
		clientType: connType,
		state:      ConnStateReady,
		opts:       ClientOptions{},
		errc:       make(chan error),
	}
	c.Init(opts...)
	once.Do(func() {
		c.init()
	})

	return c
}

// Init the options.
func (c *Client) Init(opts ...ClientOption) error {
	for _, o := range opts {
		o(&c.opts)
	}
	return c.initOptions()
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
	conn, err := quic.DialAddrContext(ctx, addr, c.opts.TLSConfig, c.opts.QuicConfig)
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
		c.opts.ObserveDataTags,
		c.opts.Credential.Name(),
		c.opts.Credential.Payload(),
	)
	if _, err := c.fs.WriteFrame(handshake); err != nil {
		c.state = ConnStateDisconnected
		return err
	}

	c.state = ConnStateConnected
	c.localAddr = c.conn.LocalAddr().String()

	c.logger.Printf("%s❤️  [%s][%s](%s) is connected to YoMo-Zipper %s", ClientLogPrefix, c.name, c.clientID, c.localAddr, addr)

	// receiving frames
	go func() {
		closeConn, closeClient, err := c.handleFrame()
		c.logger.Debugf("%shandleFrame: %v, %v, %T, %v", ClientLogPrefix, closeConn, closeClient, err, err)

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
				c.logger.Warnf("%s%v", ClientLogPrefix, err)
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
		c.logger.Debugf("%stype=%s, frame=%# x", ClientLogPrefix, frameType, frame.Shortly(f.Encode()))
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
				c.logger.Debugf("%sreceive DataFrame, tag=%#x, tid=%s, carry=%# x", ClientLogPrefix, v.GetDataTag(), v.TransactionID(), v.GetCarriage())
				if c.processor == nil {
					c.logger.Warnf("%sprocessor is nil", ClientLogPrefix)
				} else {
					c.processor(v)
				}
			}
		case frame.TagOfBackflowFrame:
			if v, ok := f.(*frame.BackflowFrame); ok {
				c.logger.Debugf("%sreceive BackflowFrame, tag=%#x, carry=%# x", ClientLogPrefix, v.GetDataTag(), v.GetCarriage())
				if c.receiver == nil {
					c.logger.Warnf("%sreceiver is nil", ClientLogPrefix)
				} else {
					c.receiver(v)
				}
			}
		default:
			c.logger.Warnf("%sunknown or unsupported frame %#x", ClientLogPrefix, frameType)
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
	c.logger.Printf("%s💔 close the connection, name:%s, id:%s, addr:%s", ClientLogPrefix, c.name, c.clientID, c.addr)

	// close error channel so that close handler function will be called
	close(c.errc)

	c.state = ConnStateClosed
	return nil
}

// WriteFrame writes a frame to the connection, gurantee threadsafe.
func (c *Client) WriteFrame(frm frame.Frame) error {
	c.logger.Debugf("%s[%s](%s)@%s WriteFrame() will write frame: %s", ClientLogPrefix, c.name, c.localAddr, c.state, frm.Type())

	if c.state != ConnStateConnected {
		return errors.New("client connection isn't connected")
	}

	if _, err := c.fs.WriteFrame(frm); err != nil {
		return err
	}

	return nil
}

// SetDataFrameObserver sets the data frame handler.
func (c *Client) SetDataFrameObserver(fn func(*frame.DataFrame)) {
	c.processor = fn
	c.logger.Debugf("%sSetDataFrameObserver(%v)", ClientLogPrefix, c.processor)
}

// SetBackflowFrameObserver sets the backflow frame handler.
func (c *Client) SetBackflowFrameObserver(fn func(*frame.BackflowFrame)) {
	c.receiver = fn
	c.logger.Debugf("%sSetBackflowFrameObserver(%v)", ClientLogPrefix, c.receiver)
}

// reconnect the connection between client and server.
func (c *Client) reconnect(ctx context.Context, addr string) {
	t := time.NewTicker(1 * time.Second)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			c.logger.Debugf("%s[%s](%s) context.Done()", ClientLogPrefix, c.name, c.localAddr)
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
				c.logger.Printf("%s[%s][%s](%s) is reconnecting to YoMo-Zipper %s...", ClientLogPrefix, c.name, c.clientID, c.localAddr, addr)
				err := c.connect(ctx, addr)
				if err != nil {
					c.logger.Errorf("%s[%s][%s](%s) reconnect error:%v", ClientLogPrefix, c.name, c.clientID, c.localAddr, err)
				}
			}
		}
	}
}

func (c *Client) init() {
	// // tracing
	// _, _, err := tracing.NewTracerProvider(c.name)
	// if err != nil {
	// 	logger.Errorf("tracing: %v", err)
	// }
}

// ServerAddr returns the address of the server.
func (c *Client) ServerAddr() string {
	return c.addr
}

// initOptions init options defaults
func (c *Client) initOptions() error {
	// logger
	if c.logger == nil {
		if c.opts.Logger != nil {
			c.logger = c.opts.Logger
		} else {
			c.logger = logger.Default()
		}
	}
	// observe tag list
	if c.opts.ObserveDataTags == nil {
		c.opts.ObserveDataTags = make([]byte, 0)
	}
	// credential
	if c.opts.Credential == nil {
		c.opts.Credential = auth.NewCredential("")
	}
	// tls config
	if c.opts.TLSConfig == nil {
		tc, err := pkgtls.CreateClientTLSConfig()
		if err != nil {
			c.logger.Errorf("%sCreateClientTLSConfig: %v", ClientLogPrefix, err)
			return err
		}
		c.opts.TLSConfig = tc
	}
	// quic config
	if c.opts.QuicConfig == nil {
		c.opts.QuicConfig = &quic.Config{
			Versions:                       []quic.VersionNumber{quic.Version1, quic.VersionDraft29},
			MaxIdleTimeout:                 time.Second * 40,
			KeepAlivePeriod:                time.Second * 20,
			MaxIncomingStreams:             1000,
			MaxIncomingUniStreams:          1000,
			HandshakeIdleTimeout:           time.Second * 3,
			InitialStreamReceiveWindow:     1024 * 1024 * 2,
			InitialConnectionReceiveWindow: 1024 * 1024 * 2,
			TokenStore:                     quic.NewLRUTokenStore(10, 5),
			// DisablePathMTUDiscovery:        true,
		}
	}
	// credential
	if c.opts.Credential != nil {
		c.logger.Printf("%suse credential: [%s]", ClientLogPrefix, c.opts.Credential.Name())
	}

	return nil
}

// SetObserveDataTags set the data tag list that will be observed.
// Deprecated: use yomo.WithObserveDataTags instead
func (c *Client) SetObserveDataTags(tag ...byte) {
	c.opts.ObserveDataTags = append(c.opts.ObserveDataTags, tag...)
}

// Logger get client's logger instance, you can customize this using `yomo.WithLogger`
func (c *Client) Logger() log.Logger {
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
