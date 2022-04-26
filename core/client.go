package core

import (
	"context"
	"errors"
	"fmt"
	"net"

	"sync"
	"time"

	"github.com/lucas-clemente/quic-go"
	"github.com/yomorun/yomo/core/auth"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/log"
	"github.com/yomorun/yomo/core/yerr"
	"github.com/yomorun/yomo/pkg/logger"
	pkgtls "github.com/yomorun/yomo/pkg/tls"
)

// ClientOption YoMo client options
type ClientOption func(*ClientOptions)

// ConnState describes the state of the connection.
type ConnState = string

// Client is the abstraction of a YoMo-Client. a YoMo-Client can be
// Source, Upstream Zipper or StreamFunction.
type Client struct {
	name       string                     // name of the client
	clientType ClientType                 // type of the connection
	conn       quic.Connection            // quic connection
	stream     quic.Stream                // quic stream
	state      ConnState                  // state of the connection
	processor  func(*frame.DataFrame)     // functions to invoke when data arrived
	receiver   func(*frame.BackflowFrame) // functions to invoke when data is processed
	addr       string                     // the address of server connected to
	mu         sync.Mutex
	opts       ClientOptions
	localAddr  string // client local addr, it will be changed on reconnect
	logger     log.Logger
	errc       chan error
	closec     chan bool
	closed     bool
}

// NewClient creates a new YoMo-Client.
func NewClient(appName string, connType ClientType, opts ...ClientOption) *Client {
	c := &Client{
		name:       appName,
		clientType: connType,
		state:      ConnStateReady,
		opts:       ClientOptions{},
		errc:       make(chan error),
		closec:     make(chan bool),
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
	if err := c.connect(ctx, addr); err != nil {
		return err
	}

	return nil
}

func (c *Client) connect(ctx context.Context, addr string) error {
	c.addr = addr
	c.state = ConnStateConnecting

	// create quic connection
	conn, err := quic.DialAddrContext(ctx, addr, c.opts.TLSConfig, c.opts.QuicConfig)
	if err != nil {
		c.state = ConnStateDisconnected
		return err
	}

	// quic stream
	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		c.state = ConnStateDisconnected
		return err
	}

	c.stream = stream
	c.conn = conn

	c.state = ConnStateAuthenticating
	// send handshake
	handshake := frame.NewHandshakeFrame(
		c.name,
		byte(c.clientType),
		c.opts.ObserveDataTags,
		c.opts.Credential.Name(),
		c.opts.Credential.Payload(),
	)
	err = c.WriteFrame(handshake)
	if err != nil {
		c.state = ConnStateRejected
		return err
	}
	c.state = ConnStateConnected
	c.localAddr = c.conn.LocalAddr().String()

	c.logger.Printf("%s❤️  [%s](%s) is connected to YoMo-Zipper %s", ClientLogPrefix, c.name, c.localAddr, addr)

	// receiving frames
	go c.handleFrame()

	return nil
}

// handleFrame handles the logic when receiving frame from server.
func (c *Client) handleFrame() {
	// transform raw QUIC stream to wire format
	fs := NewFrameStream(c.stream)
	for {
		c.logger.Debugf("%shandleFrame connection state=%v", ClientLogPrefix, c.state)
		// this will block until a frame is received
		f, err := fs.ReadFrame()
		if err != nil {
			defer c.stream.Close()
			// defer c.conn.CloseWithError(0xD0, err.Error())

			c.logger.Debugf("%shandleFrame(): %T | %v", ClientLogPrefix, err, err)
			if e, ok := err.(*quic.IdleTimeoutError); ok {
				c.logger.Errorf("%sconnection timeout, err=%v, zipper=%s", ClientLogPrefix, e, c.addr)
				c.setState(ConnStateDisconnected)
			} else if e, ok := err.(*quic.ApplicationError); ok {
				c.logger.Infof("%sapplication error, err=%v, errcode=%v", ClientLogPrefix, e, e.ErrorCode)
				if yerr.Is(e.ErrorCode, yerr.ErrorCodeRejected) {
					// if connection is rejected(eg: authenticate fails) from server
					c.logger.Errorf("%sIllegal client, server rejected.", ClientLogPrefix)
					c.setState(ConnStateRejected)
					break
				} else if yerr.Is(e.ErrorCode, yerr.ErrorCodeClientAbort) {
					// client abort
					c.logger.Infof("%sclient close the connection", ClientLogPrefix)
					c.setState(ConnStateAborted)
					break
				} else if yerr.Is(e.ErrorCode, yerr.ErrorCodeGoaway) {
					// server goaway
					c.logger.Infof("%sserver goaway the connection", ClientLogPrefix)
					c.setState(ConnStateGoaway)
					break
				}
			} else if errors.Is(err, net.ErrClosed) {
				// if client close the connection, net.ErrClosed will be raise
				c.logger.Errorf("%sconnection is closed, err=%v", ClientLogPrefix, err)
				c.setState(ConnStateDisconnected)
				// by quic-go IdleTimeoutError after connection's KeepAlive config.
				break
			} else {
				// any error occurred, we should close the stream
				// after this, conn.AcceptStream() will raise the error
				c.setState(ConnStateClosed)
				c.conn.CloseWithError(yerr.To(yerr.ErrorCodeUnknown), err.Error())
				c.logger.Errorf("%sunknown error occurred, err=%v, state=%v", ClientLogPrefix, err, c.getState())
				break
			}
		}
		if f == nil {
			break
		}
		// read frame
		// first, get frame type
		frameType := f.Type()
		c.logger.Debugf("%stype=%s, frame=%# x", ClientLogPrefix, frameType, frame.Shortly(f.Encode()))
		switch frameType {
		case frame.TagOfHandshakeFrame:
			if v, ok := f.(*frame.HandshakeFrame); ok {
				c.logger.Debugf("%sreceive HandshakeFrame, name=%v", ClientLogPrefix, v.Name)
			}
		case frame.TagOfPongFrame:
			c.setState(ConnStatePong)
		case frame.TagOfAcceptedFrame:
			c.setState(ConnStateAccepted)
		case frame.TagOfRejectedFrame:
			c.setState(ConnStateRejected)
			if v, ok := f.(*frame.RejectedFrame); ok {
				c.logger.Errorf("%s🔑 receive RejectedFrame, message=%s", ClientLogPrefix, v.Message())
				c.conn.CloseWithError(yerr.To(yerr.ErrorCodeRejected), v.Message())
				c.errc <- errors.New(v.Message())
				break
			}
		case frame.TagOfGoawayFrame:
			c.setState(ConnStateGoaway)
			if v, ok := f.(*frame.GoawayFrame); ok {
				c.logger.Errorf("%s⛔️ receive GoawayFrame, message=%s", ClientLogPrefix, v.Message())
				c.conn.CloseWithError(yerr.To(yerr.ErrorCodeGoaway), v.Message())
				c.errc <- errors.New(v.Message())
				break
			}
		case frame.TagOfDataFrame: // DataFrame carries user's data
			if v, ok := f.(*frame.DataFrame); ok {
				c.setState(ConnStateTransportData)
				c.logger.Debugf("%sreceive DataFrame, tag=%# x, tid=%s, carry=%# x", ClientLogPrefix, v.GetDataTag(), v.TransactionID(), v.GetCarriage())
				if c.processor == nil {
					c.logger.Warnf("%sprocessor is nil", ClientLogPrefix)
				} else {
					// TODO: should c.processor accept a DataFrame as parameter?
					// c.processor(v.GetDataTagID(), v.GetCarriage(), v.GetMetaFrame())
					c.processor(v)
				}
			}
		case frame.TagOfBackflowFrame:
			if v, ok := f.(*frame.BackflowFrame); ok {
				c.setState(ConnStateTransportData)
				c.logger.Debugf("%sreceive BackflowFrame, tag=%# x, carry=%# x", ClientLogPrefix, v.GetDataTag(), v.GetCarriage())
				if c.receiver == nil {
					c.logger.Warnf("%sreceiver is nil", ClientLogPrefix)
				} else {
					c.receiver(v)
				}
			}
		default:
			c.logger.Errorf("%sunknown signal", ClientLogPrefix)
		}
	}
}

// Close the client.
func (c *Client) Close() (err error) {
	if c.conn != nil {
		c.logger.Printf("%sclose the connection, name:%s, addr:%s", ClientLogPrefix, c.name, c.conn.RemoteAddr().String())
	}
	if c.stream != nil {
		err = c.stream.Close()
		if err != nil {
			c.logger.Errorf("%s stream.Close(): %v", ClientLogPrefix, err)
		}
	}
	if c.conn != nil {
		err = c.conn.CloseWithError(0, "client-ask-to-close-this-connection")
		if err != nil {
			c.logger.Errorf("%s connection.Close(): %v", ClientLogPrefix, err)
		}
	}
	// close channel
	c.mu.Lock()
	if !c.closed {
		close(c.errc)
		close(c.closec)
		c.closed = true
	}
	c.mu.Unlock()

	return err
}

// EnableDebug enables the development model for logging.
// func (c *Client) EnableDebug() {
// 	logger.EnableDebug()
// }

// WriteFrame writes a frame to the connection, gurantee threadsafe.
func (c *Client) WriteFrame(frm frame.Frame) error {
	// write on QUIC stream
	if c.stream == nil {
		return errors.New("stream is nil")
	}
	if c.state == ConnStateDisconnected || c.state == ConnStateRejected {
		return fmt.Errorf("client connection state is %s", c.state)
	}
	c.logger.Debugf("%s[%s](%s)@%s WriteFrame() will write frame: %s", ClientLogPrefix, c.name, c.localAddr, c.state, frm.Type())

	data := frm.Encode()
	// emit raw bytes of Frame
	c.mu.Lock()
	n, err := c.stream.Write(data)
	c.mu.Unlock()
	c.logger.Debugf("%sWriteFrame() wrote n=%d, data=%# x", ClientLogPrefix, n, frame.Shortly(data))
	if err != nil {
		c.setState(ConnStateDisconnected)
		// c.state = ConnStateDisconnected
		if e, ok := err.(*quic.IdleTimeoutError); ok {
			c.logger.Errorf("%sWriteFrame() connection timeout, err=%v", ClientLogPrefix, e)
		} else {
			c.logger.Errorf("%sWriteFrame() wrote error=%v", ClientLogPrefix, err)
			return err
		}
	}
	if n != len(data) {
		err := errors.New("[client] yomo Client .Write() wroten error")
		c.logger.Errorf("%s error:%v", ClientLogPrefix, err)
		return err
	}
	return err
}

// update connection state
func (c *Client) setState(state ConnState) {
	c.logger.Debugf("setState to:%s", state)
	c.mu.Lock()
	c.state = state
	c.mu.Unlock()
}

// getState get connection state
func (c *Client) getState() ConnState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

// update connection local addr
func (c *Client) setLocalAddr(addr string) {
	c.mu.Lock()
	c.localAddr = addr
	c.mu.Unlock()
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
		case <-c.closec:
			c.logger.Debugf("%s[%s](%s) close channel", ClientLogPrefix, c.name, c.localAddr)
			return
		case <-t.C:
			if c.getState() == ConnStateDisconnected {
				c.logger.Printf("%s[%s](%s) is reconnecting to YoMo-Zipper %s...", ClientLogPrefix, c.name, c.localAddr, addr)
				err := c.connect(ctx, addr)
				if err != nil {
					c.logger.Errorf("%s[%s](%s) reconnect error:%v", ClientLogPrefix, c.name, c.localAddr, err)
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
			KeepAlive:                      true,
			MaxIncomingStreams:             1000,
			MaxIncomingUniStreams:          1000,
			HandshakeIdleTimeout:           time.Second * 3,
			InitialStreamReceiveWindow:     1024 * 1024 * 2,
			InitialConnectionReceiveWindow: 1024 * 1024 * 2,
			TokenStore:                     quic.NewLRUTokenStore(10, 5),
			DisablePathMTUDiscovery:        true,
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
	if fn != nil {
		go func() {
			err := <-c.errc
			if err != nil {
				fn(err)
			}
		}()
	}
}
