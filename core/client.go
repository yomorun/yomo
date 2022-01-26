package core

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/lucas-clemente/quic-go"
	"github.com/yomorun/yomo/core/auth"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/pkg/logger"
	pkgtls "github.com/yomorun/yomo/pkg/tls"
)

type ClientOption func(*ClientOptions)

// ConnState describes the state of the connection.
type ConnState = string

// Client is the abstraction of a YoMo-Client. a YoMo-Client can be
// Source, Upstream Zipper or StreamFunction.
type Client struct {
	name       string                 // name of the client
	clientType ClientType             // type of the connection
	session    quic.Session           // quic session
	stream     quic.Stream            // quic stream
	state      ConnState              // state of the connection
	processor  func(*frame.DataFrame) // functions to invoke when data arrived
	addr       string                 // the address of server connected to
	mu         sync.Mutex
	opts       ClientOptions
	localAddr  string // client local addr, it will be changed on reconnect
}

// NewClient creates a new YoMo-Client.
func NewClient(appName string, connType ClientType, opts ...ClientOption) *Client {
	c := &Client{
		name:       appName,
		clientType: connType,
		state:      ConnStateReady,
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
	session, err := quic.DialAddrContext(ctx, addr, c.opts.TLSConfig, c.opts.QuicConfig)
	if err != nil {
		c.state = ConnStateDisconnected
		return err
	}

	// quic stream
	stream, err := session.OpenStreamSync(ctx)
	if err != nil {
		c.state = ConnStateDisconnected
		return err
	}

	c.stream = stream
	c.session = session

	c.state = ConnStateAuthenticating
	// send handshake
	handshake := frame.NewHandshakeFrame(
		c.name,
		byte(c.clientType),
		c.opts.Credential.AppID(),
		byte(c.opts.Credential.Type()),
		c.opts.Credential.Payload(),
	)
	err = c.WriteFrame(handshake)
	if err != nil {
		c.state = ConnStateRejected
		return err
	}
	c.state = ConnStateConnected
	c.localAddr = c.session.LocalAddr().String()

	logger.Printf("%s❤️  [%s](%s) is connected to YoMo-Zipper %s", ClientLogPrefix, c.name, c.localAddr, addr)

	// receiving frames
	go c.handleFrame()

	return nil
}

// handleFrame handles the logic when receiving frame from server.
func (c *Client) handleFrame() {
	// transform raw QUIC stream to wire format
	fs := NewFrameStream(c.stream)
	for {
		logger.Debugf("%shandleFrame connection state=%v", ClientLogPrefix, c.state)
		// this will block until a frame is received
		f, err := fs.ReadFrame()
		if err != nil {
			defer c.stream.Close()
			defer c.session.CloseWithError(0xD0, err.Error())
			defer c.setState(ConnStateDisconnected)

			logger.Errorf("%shandleFrame.ReadFrame(): %T %v", ClientLogPrefix, err, err)
			if e, ok := err.(*quic.IdleTimeoutError); ok {
				logger.Errorf("%sconnection timeout, err=%v, zipper=%s", ClientLogPrefix, e, c.addr)
			} else if e, ok := err.(*quic.ApplicationError); ok {
				logger.Errorf("%sapplication error, err=%v, errcode=%v", ClientLogPrefix, e, e.ErrorCode)
				if e.ErrorCode == 0xCC {
					logger.Errorf("%sIllegal client, server rejected.", ClientLogPrefix)
					// TODO: stop reconnect policy will be much better than exit process.
					os.Exit(0xCC)
				}
			} else if errors.Is(err, net.ErrClosed) {
				// if client close the connection, net.ErrClosed will be raise
				logger.Errorf("%sconnection is closed, err=%v", ClientLogPrefix, err)
				// by quic-go IdleTimeoutError after connection's KeepAlive config.
				break
			}
			// any error occurred, we should close the session
			// after this, session.AcceptStream() will raise the error
			// which specific in session.CloseWithError()
			break
		}

		// read frame
		// first, get frame type
		frameType := f.Type()
		logger.Debugf("%stype=%s, frame=%# x", ClientLogPrefix, frameType, f.Encode())
		switch frameType {
		case frame.TagOfPongFrame:
			c.setState(ConnStatePong)
		case frame.TagOfAcceptedFrame:
			c.setState(ConnStateAccepted)
		case frame.TagOfRejectedFrame:
			c.setState(ConnStateRejected)
			c.Close()
		case frame.TagOfDataFrame: // DataFrame carries user's data
			if v, ok := f.(*frame.DataFrame); ok {
				c.setState(ConnStateTransportData)
				logger.Debugf("%sreceive DataFrame, tag=%# x, tid=%s, carry=%# x", ClientLogPrefix, v.GetDataTag(), v.TransactionID(), v.GetCarriage())
				if c.processor == nil {
					logger.Warnf("%sprocessor is nil", ClientLogPrefix)
				} else {
					// TODO: should c.processor accept a DataFrame as parameter?
					// c.processor(v.GetDataTagID(), v.GetCarriage(), v.GetMetaFrame())
					c.processor(v)
				}
			}
		default:
			logger.Errorf("%sunknown signal", ClientLogPrefix)
		}
	}
}

// Close the client.
func (c *Client) Close() (err error) {
	logger.Debugf("%sclose the connection", ClientLogPrefix)
	if c.stream != nil {
		err = c.stream.Close()
		if err != nil {
			logger.Errorf("%s stream.Close(): %v", ClientLogPrefix, err)
		}
	}
	if c.session != nil {
		err = c.session.CloseWithError(255, "client.session closed")
		if err != nil {
			logger.Errorf("%s session.Close(): %v", ClientLogPrefix, err)
		}
	}

	return err
}

// EnableDebug enables the development model for logging.
func (c *Client) EnableDebug() {
	logger.EnableDebug()
}

// WriteFrame writes a frame to the connection, gurantee threadsafe.
func (c *Client) WriteFrame(frm frame.Frame) error {
	// // tracing
	// if f, ok := frm.(*frame.DataFrame); ok {
	// 	span, err := tracing.NewRemoteTraceSpan(f.GetMetadata("TraceID"), f.GetMetadata("SpanID"), c.name, fmt.Sprintf("WriteFrame [%s]->[zipper]", c.name))
	// 	if err == nil {
	// 		defer span.End()
	// 	}
	// }
	// write on QUIC stream
	if c.stream == nil {
		return errors.New("stream is nil")
	}
	if c.state == ConnStateDisconnected || c.state == ConnStateRejected {
		return fmt.Errorf("client connection state is %s", c.state)
	}
	logger.Debugf("%s[%s](%s)@%s WriteFrame() will write frame: %s", ClientLogPrefix, c.name, c.localAddr, c.state, frm.Type())

	data := frm.Encode()
	// emit raw bytes of Frame
	c.mu.Lock()
	n, err := c.stream.Write(data)
	c.mu.Unlock()
	// TODO: move partial logging as a utility
	if len(data) > 16 {
		logger.Debugf("%sWriteFrame() wrote n=%d, len(data)=%d, data[:16]=%# x ...", ClientLogPrefix, n, len(data), data[:16])
	} else {
		logger.Debugf("%sWriteFrame() wrote n=%d, len(data)=%d, data=%# x", ClientLogPrefix, n, len(data), data)
	}
	if err != nil {
		c.setState(ConnStateDisconnected)
		// c.state = ConnStateDisconnected
		if e, ok := err.(*quic.IdleTimeoutError); ok {
			logger.Errorf("%sWriteFrame() connection timeout, err=%v", ClientLogPrefix, e)
		} else {
			logger.Errorf("%sWriteFrame() wrote error=%v", ClientLogPrefix, err)
			return err
		}
	}
	if n != len(data) {
		err := errors.New("[client] yomo Client .Write() wroten error")
		logger.Errorf("%s error:%v", ClientLogPrefix, err)
		return err
	}
	return err
}

// update connection state
func (c *Client) setState(state ConnState) {
	c.mu.Lock()
	c.state = state
	c.mu.Unlock()
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
	logger.Debugf("%sSetDataFrameObserver(%v)", ClientLogPrefix, c.processor)
}

// reconnect the connection between client and server.
func (c *Client) reconnect(ctx context.Context, addr string) {
	t := time.NewTicker(1 * time.Second)
	defer t.Stop()
	for range t.C {
		if c.state == ConnStateDisconnected {
			fmt.Printf("%s[%s](%s) is retrying to YoMo-Zipper %s...\n", ClientLogPrefix, c.name, c.localAddr, addr)
			err := c.connect(ctx, addr)
			if err != nil {
				logger.Errorf("%s[%s](%s) reconnect error:%v", ClientLogPrefix, c.name, c.localAddr, err)
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
	// credential
	if c.opts.Credential == nil {
		c.opts.Credential = auth.NewCredendialNone()
	}
	// tls config
	if c.opts.TLSConfig == nil {
		tc, err := pkgtls.CreateClientTLSConfig()
		if err != nil {
			logger.Errorf("%sCreateClientTLSConfig: %v", ClientLogPrefix, err)
			return err
		}
		c.opts.TLSConfig = tc
	}
	// quic config
	if c.opts.QuicConfig == nil {
		c.opts.QuicConfig = &quic.Config{
			Versions:                       []quic.VersionNumber{quic.Version1, quic.VersionDraft29},
			MaxIdleTimeout:                 time.Second * 10,
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
		logger.Printf("%suse credential: [%s]", ClientLogPrefix, c.opts.Credential.Type())
	}

	return nil
}
