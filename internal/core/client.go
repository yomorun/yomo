package core

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/lucas-clemente/quic-go"
	"github.com/yomorun/yomo/internal/frame"
	"github.com/yomorun/yomo/logger"
)

type ConnState = string

const (
	ConnStateDisconnected   ConnState = "Disconnected"
	ConnStateConnecting     ConnState = "Connecting"
	ConnStateAuthenticating ConnState = "Authenticating"
	ConnStateAccepted       ConnState = "Accepted"
	ConnStateRejected       ConnState = "Rejected"
	ConnStatePing           ConnState = "Ping"
	ConnStatePong           ConnState = "Pong"
	ConnStateTransportData  ConnState = "TransportData"
)

// Client is the implementation of Client interface.
type Client struct {
	token             string
	connType          ConnectionType
	session           quic.Session
	stream            quic.Stream
	state             string
	lastFrameSentTick time.Time
	mu                sync.Mutex
}

// New creates a new client.
func NewClient(appName string, connType ConnectionType) *Client {
	c := &Client{
		token:    appName,
		connType: connType,
		state:    ConnStateDisconnected,
	}

	return c
}

// Connect connects to quic server
func (c *Client) Connect(ctx context.Context, addr string) error {
	c.state = ConnStateConnecting
	logger.Printf("Connecting to YoMo-Zipper %s...", addr)

	// connect to quic server
	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"spdy/3", "h2", "hq-29"},
		ClientSessionCache: tls.NewLRUClientSessionCache(1),
	}
	quicConf := &quic.Config{
		Versions:                       []quic.VersionNumber{quic.Version1},
		MaxIdleTimeout:                 time.Second * 60,
		KeepAlive:                      true,
		MaxIncomingStreams:             1000000,
		MaxIncomingUniStreams:          1000000,
		HandshakeIdleTimeout:           time.Second * 10,
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
	c.handleFrame()
	// send ping to zipper.
	// c.ping()
	// check if receiving the pong from zipper.
	// c.Healthcheck()
	return nil
}

// handleFrame handles the logic when receiving frame from server.
func (c *Client) handleFrame() {
	go func() {
		for {
			f, err := ParseFrame(c.stream)
			if err != nil {
				logger.Error("[client] on [ParseFrame]", "err", err)
				if errors.Is(err, net.ErrClosed) {
					// if client close the connection, net.ErrClosed will be raise
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
			logger.Debug("[client]", "type", frameType.String(), "frame", logger.BytesString(f.Encode()))
			switch frameType {
			case frame.TagOfPongFrame:
				// c.heartbeat <- true
				c.setState(ConnStatePong)

			case frame.TagOfAcceptedFrame:
				// create stream
				// if c.conn.Type == core.ConnTypeSource || c.conn.Type == core.ConnTypeUpstreamZipper {
				// 	stream, err := c.Session.CreateStream(context.Background())
				// 	if err != nil {
				// 		logger.Error("[client] session.CreateStream Error:", "err", err)
				// 		break
				// 	}

				// 	c.Stream = core.NewFrameStream(stream)
				// }
				// accepted <- true
				c.setState(ConnStateAccepted)
				// if err := c.OnAccepted(); err != nil {
				// 	c.Close()
				// 	break
				// }
				// TODO: accepted
			case frame.TagOfRejectedFrame:
				// if c.conn.Type == core.ConnTypeStreamFunction {
				// 	logger.Warn("[client] the connection was rejected by zipper, please check if the function name matches the one in zipper config.")
				// } else {
				// 	logger.Warn("[client] the connection was rejected by zipper.")
				// }
				c.setState(ConnStateRejected)
				c.Close()
				break
				// TODO: data frame 独立处理
			case frame.TagOfDataFrame:
				// if c.conn.Type == core.ConnTypeStreamFunction {
				// 	logger.Warn("[client] the connection was rejected by zipper, please check if the function name matches the one in zipper config.")
				// } else {
				// 	logger.Warn("[client] the connection was rejected by zipper.")
				// }
				c.setState(ConnStateTransportData)
				data := f.Encode()
				c.OnData(data)
			default:
				logger.Debug("[client] unknown signal.", "frame", logger.BytesString(f.Encode()))
			}
		}
	}()
}

// Ping sends the PingFrame to YoMo-Zipper in every 3s.
// func (c *Client) ping() {
// 	go func(c *Client) {
// 		t := time.NewTicker(3 * time.Second)
// 		for {
// 			select {
// 			case <-t.C:
// 				_, err := c.signal.WriteFrame(frame.NewPingFrame())
// 				logger.Info("Send Ping to zipper.")
// 				if err != nil {
// 					if err.Error() == quic.ErrConnectionClosed {
// 						logger.Print("[client] ❌ the zipper was offline.")
// 					} else {
// 						// other errors.
// 						logger.Error("[client] ❌ sent Ping to zipper failed.", "err", err)
// 					}

// 					t.Stop()
// 					break
// 				}
// 			}
// 		}
// 	}(c)
// }

// Retry the connection between client and server.
// func (c *Impl) Retry() {
// 	for {
// 		logger.Debug("[client] retry to connect the YoMo-Zipper...", "addr", getServerAddr(c.serverIP, c.serverPort))
// 		_, err := c.BaseConnect(c.serverIP, c.serverPort)
// 		if err == nil {
// 			break
// 		}

// 		time.Sleep(3 * time.Second)
// 	}
// }

// RetryWithCount the connection with a certain count.
// func (c *Impl) RetryWithCount(count int) bool {
// 	for i := 0; i < count; i++ {
// 		logger.Debug("[client] retry to connect the YoMo-Zipper with count...", "addr", getServerAddr(c.serverIP, c.serverPort), "count", count)
// 		_, err := c.BaseConnect(c.serverIP, c.serverPort)
// 		if err == nil {
// 			return true
// 		}

// 		time.Sleep(3 * time.Second)
// 	}
// 	return false
// }

// Close the client.
func (c *Client) Close() error {
	logger.Debug("[client] close the connection")
	if c.stream != nil {
		err := c.stream.Close()
		if err != nil {
			logger.Errorf("close(): %v", err)
			return err
		}
	}
	if c.session != nil {
		err := c.session.CloseWithError(255, "client.session closed")
		if err != nil {
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
		return errors.New("[client] Stream is nil")
	}
	logger.Debug("[client] WriteFrame() will write frame: %# x", frame.Type())
	c.lastFrameSentTick = time.Now()
	data := frame.Encode()
	n, err := c.stream.Write(data)
	if len(data) > 256 {
		logger.Debugf("[client] WriteFrame() wrote n=%d, len(data)=%d", n, len(data))
	} else {
		logger.Debugf("[client] WriteFrame() wrote n=%d, data=%# x", n, data)
	}
	if err != nil {
		logger.Errorf("[client] WriteFrame() wrote error=%v", err)
		// 发送数据时出错
		return err
	}
	if n != len(data) {
		// 发送的数据不完整
		return errors.New("[client] yomo Client .Write() wroten error")
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

func (c *Client) OnData(data []byte) error {
	logger.Debugf("[client] OnData: data=%v", data)
	return nil
}

// SendSignal sends the signal to client.
// func (c *Client) SendSignal(f frame.Frame) error {
// 	if c.signal == nil {
// 		return errors.New("Signal is nil")
// 	}

// 	_, err := c.signal.WriteFrame(f)
// 	return err
// }

// Healthcheck checks if peer is online by heartbeat.
// func (c *Client) Healthcheck() {
// 	go func() {
// 		// receive heartbeat
// 		defer c.Close()
// 	loop:
// 		for {
// 			select {
// 			case _, ok := <-c.heartbeat:
// 				if !ok {
// 					break loop
// 				}
// 				if c.OnHeartbeatReceived != nil {
// 					c.OnHeartbeatReceived()
// 				}

// 			case <-time.After(HeartbeatTimeOut):
// 				// didn't receive the heartbeat after a certain duration, call the callback function when expired.
// 				if c.OnHeartbeatExpired != nil {
// 					c.OnHeartbeatExpired()
// 				}

// 				break loop
// 			}
// 		}
// 	}()
// }
