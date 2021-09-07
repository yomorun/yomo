package core

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/lucas-clemente/quic-go"
	"github.com/yomorun/yomo/internal/frame"
	"github.com/yomorun/yomo/logger"
)

const (
	DefaultRetries = 3 // 3sec*120 = 1hour
)

type ConnState = string

// Client is the implementation of Client interface.
type Client struct {
	token             string
	connType          ConnectionType
	session           quic.Session
	stream            quic.Stream
	state             string
	lastFrameSentTick time.Time
	processor         func(byte, []byte)
	mu                sync.Mutex
}

// New creates a new client.
func NewClient(appName string, connType ConnectionType) *Client {
	c := &Client{
		token:    appName,
		connType: connType,
		state:    ConnStateReady,
	}

	return c
}

// Connect connects to quic server
func (c *Client) Connect(ctx context.Context, addr string) error {
	c.state = ConnStateConnecting

	// connect to quic server
	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"spdy/3", "h2", "hq-29"},
		ClientSessionCache: tls.NewLRUClientSessionCache(1),
	}
	quicConf := &quic.Config{
		Versions:                       []quic.VersionNumber{quic.Version1, quic.VersionDraft29},
		MaxIdleTimeout:                 time.Second * 3,
		KeepAlive:                      true,
		MaxIncomingStreams:             100000,
		MaxIncomingUniStreams:          100000,
		HandshakeIdleTimeout:           time.Second * 3,
		InitialStreamReceiveWindow:     1024 * 1024 * 2,
		InitialConnectionReceiveWindow: 1024 * 1024 * 2,
		TokenStore:                     quic.NewLRUTokenStore(1, 1),
		DisablePathMTUDiscovery:        true,
	}
	// quic session
	session, err := quic.DialAddr(addr, tlsConf, quicConf)
	if err != nil {
		c.state = ConnStateDisconnected
		return err
	}

	// quic stream
	// old: 	return c.session.OpenStream()
	stream, err := session.OpenStreamSync(ctx)
	if err != nil {
		c.state = ConnStateDisconnected
		return err
	}

	// set session and signal
	c.stream = stream
	// c.signal = NewFrameStream(stream)
	c.session = session
	c.state = ConnStateAuthenticating
	// handshake frame
	handshake := frame.NewHandshakeFrame(c.token, byte(c.connType))
	c.WriteFrame(handshake)
	go c.handleFrame()

	c.state = ConnStateConnected
	logger.Printf("%s[%s] is connected to YoMo-Zipper %s", ClientLogPrefix, c.token, addr)
	// reconnect
	go c.reconnect(ctx, addr)

	return nil
}

// handleFrame handles the logic when receiving frame from server.
func (c *Client) handleFrame() {
	go func() {
		fs := NewFrameStream(c.stream)
		for {
			logger.Infof("%sconnection state=%v", ClientLogPrefix, c.state)
			f, err := fs.ReadFrame()
			if err != nil {
				c.setState(ConnStateDisconnected)
				logger.Errorf("%shandleFrame.ReadFrame(): %T %v", ClientLogPrefix, err, err)
				if e, ok := err.(*quic.IdleTimeoutError); ok {
					logger.Errorf("%sconnection timeout, err=%v", ClientLogPrefix, e)
					// c.reconnect(context.Background(), c.zipperEndpoint)
				} else if e, ok := err.(*quic.ApplicationError); ok {
					logger.Errorf("%sapplication error, err=%#v", ClientLogPrefix, e)
				} else if errors.Is(err, net.ErrClosed) {
					// if client close the connection, net.ErrClosed will be raise
					logger.Errorf("%sconnection is closed, err=%v", ClientLogPrefix, err)
					// by quic-go IdleTimeoutError after connection's KeepAlive config.
					break
				}
				// any error occurred, we should close the session
				// after this, session.AcceptStream() will raise the error
				// which specific in session.CloseWithError()
				c.stream.Close()
				c.session.CloseWithError(0xCC, err.Error())
				break
			}
			// frame type
			frameType := f.Type()
			logger.Debugf("%stype=%s, frame=%# x", ClientLogPrefix, frameType, logger.BytesString(f.Encode()))
			switch frameType {
			case frame.TagOfPongFrame:
				// TODO: pong frame
				// c.heartbeat <- true
				c.setState(ConnStatePong)

			case frame.TagOfAcceptedFrame:
				// TODO: accepted
				c.setState(ConnStateAccepted)
			case frame.TagOfRejectedFrame:
				// TODO: rejected frame
				c.setState(ConnStateRejected)
				c.Close()
				break
			case frame.TagOfDataFrame:
				if v, ok := f.(*frame.DataFrame); ok {
					c.setState(ConnStateTransportData)
					logger.Debugf("%sreceive DataFrame, tag=%#x, tid=%s, issuer=%s, carry=%s", ClientLogPrefix, v.GetDataTagID(), v.TransactionID(), v.Issuer(), v.GetCarriage())
					if c.processor == nil {
						logger.Warnf("%sprocessor is nil", ClientLogPrefix)
					} else {
						go c.processor(v.GetDataTagID(), v.GetCarriage())
					}
				}
			default:
				logger.Errorf("%sunknown signal", ClientLogPrefix)
			}
		}
	}()
}

// Close the client.
func (c *Client) Close() error {
	logger.Debugf("%sclose the connection", ClientLogPrefix)
	if c.stream != nil {
		err := c.stream.Close()
		if err != nil {
			logger.Errorf("%s stream.Close(): %v", ClientLogPrefix, err)
			return err
		}
	}
	if c.session != nil {
		err := c.session.CloseWithError(255, "client.session closed")
		if err != nil {
			logger.Errorf("%s session.Close(): %v", ClientLogPrefix, err)
			return err
		}
	}

	return nil
}

// EnableDebug enables the development model for logging.
func (c *Client) EnableDebug() {
	logger.EnableDebug()
}

func (c *Client) WriteFrame(frame frame.Frame) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stream == nil {
		return errors.New("Stream is nil")
	}
	logger.Debugf("%sWriteFrame() will write frame: %s", ClientLogPrefix, frame.Type())
	c.lastFrameSentTick = time.Now()
	data := frame.Encode()
	n, err := c.stream.Write(data)
	if len(data) > 256 {
		logger.Debugf("%sWriteFrame() wrote n=%d, len(data)=%d", ClientLogPrefix, n, len(data))
	} else {
		logger.Debugf("%sWriteFrame() wrote n=%d, data=%# x", ClientLogPrefix, n, data)
	}
	if err != nil {
		if e, ok := err.(*quic.IdleTimeoutError); ok {
			logger.Errorf("%sWriteFrame() connection timeout, err=%v", ClientLogPrefix, e)
		} else {
			logger.Errorf("%sWriteFrame() wrote error=%v", ClientLogPrefix, err)
			// 发送数据时出错
			return err
		}
	}
	if n != len(data) {
		// 发送的数据不完整
		err := errors.New("[client] yomo Client .Write() wroten error")
		logger.Errorf("%s error:%v", ClientLogPrefix, err)
		return err
	}
	return err
}

func (c *Client) setState(state ConnState) {
	c.mu.Lock()
	c.state = state
	c.mu.Unlock()
}

func (c *Client) OnAccepted(hdl func() error) error {
	if hdl != nil {
		return hdl()
	}
	return nil
}

func (c *Client) SetDataFrameObserver(fn func(byte, []byte)) {
	c.processor = fn
	logger.Debugf("%sSetDataFrameObserver(%v)", ClientLogPrefix, c.processor)

}

// reconnect the connection between client and server.
func (c *Client) reconnect(ctx context.Context, addr string) {
	t := time.NewTicker(3 * time.Second)
	for {
		select {
		case <-t.C:
			if c.state == ConnStateDisconnected {
				fmt.Printf("%s[%s] is retring to YoMo-Zipper %s...\n", ClientLogPrefix, c.token, addr)
				err := c.Connect(ctx, addr)
				if err == nil {
					break
				}
			}
		}
	}
}
