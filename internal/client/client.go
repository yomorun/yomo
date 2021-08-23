package client

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/yomorun/yomo/core/quic"
	"github.com/yomorun/yomo/internal/core"
	"github.com/yomorun/yomo/internal/frame"
	"github.com/yomorun/yomo/logger"
)

// Client is the interface for common functions of YoMo client.
type Client interface {
	// Close the client connection.
	Close() error

	// Retry the connection between client and server.
	Retry()

	// RetryWithCount retry the connection with a certain count.
	RetryWithCount(count int) bool

	// EnableDebug enables the development model for logging.
	EnableDebug()
}

// Impl is the implementation of Client interface.
type Impl struct {
	conn       *quic.Conn
	serverIP   string
	serverPort int
	Session    quic.Client
	Stream     *core.FrameStream // Stream is the stream to receive actual data from source.
	isRejected bool
}

// New creates a new client.
func New(appName string, clientType core.ConnectionType) *Impl {
	c := &Impl{
		conn: quic.NewConn(appName, clientType),
	}

	c.conn.OnHeartbeatExpired = func() {
		if c.isRejected {
			// the connection was rejected by YoMo-Zipper, don't need to re-connect.
			return
		}

		// retry the connection.
		logger.Debug("[client] heartbeat to YoMo-Zipper was expired, client will reconnect to YoMo-Zipper.", "addr", getServerAddr(c.serverIP, c.serverPort))

		// reset session to nil.
		if c.Session != nil {
			c.Session.Close()
			c.Session = nil
		}

		// reset Stream to nil.
		c.Stream = nil

		// reconnect when the heartbeat is expired.
		c.Retry()
	}

	return c
}

// BaseConnect connects to YoMo-Zipper.
// TODO: login auth
func (c *Impl) BaseConnect(ip string, port int) (*Impl, error) {
	c.serverIP = ip
	c.serverPort = port
	addr := getServerAddr(c.serverIP, c.serverPort)
	logger.Printf("Connecting to YoMo-Zipper %s...", addr)

	// connect to YoMo-Zipper
	client, err := quic.NewClient(addr)
	if err != nil {
		logger.Error("[client] quic.NewClient Error:", "err", err)
		return c, err
	}

	// create quic stream
	stream, err := client.CreateStream(context.Background())
	if err != nil {
		logger.Error("[client] CreateStream Error:", "err", err)
		return c, err
	}

	// set session and signal
	c.Session = client
	c.conn.Signal = core.NewFrameStream(stream)

	// handshake frame
	handshakeFrame := frame.NewHandshakeFrame(c.conn.Name, byte(c.conn.Type))
	logger.Debug(fmt.Sprintf("[HandshakeFrame] name=%s, type=%s ", handshakeFrame.Name, handshakeFrame.Type()))
	c.conn.Signal.WriteFrame(handshakeFrame)

	accepted := make(chan bool)

	c.handleSignal(accepted)

	// waiting when the connection is accepted.
	<-accepted

	if c.isRejected {
		logger.Printf("❌ The connection to YoMo-Zipper %s was rejected.", addr)
	} else {
		logger.Printf("✅ Connected to YoMo-Zipper %s.", addr)
	}

	// send ping to zipper.
	c.ping()

	// check if receiving the pong from zipper.
	c.conn.Healthcheck()

	return c, nil
}

// handleSignal handles the logic when receiving signal from server.
func (c *Impl) handleSignal(accepted chan bool) {
	go func() {
		for {
			f, err := c.conn.Signal.ReadFrame()
			if err != nil {
				logger.Error("[ERR] on [ParseFrame]", "err", err)
				if errors.Is(err, net.ErrClosed) {
					// if client close the connection, net.ErrClosed will be raise
					// by quic-go IdleTimeoutError after connection's KeepAlive config.
					logger.Error("[ERR] on [ParseFrame]", "err", net.ErrClosed)
					break
				}
				// any error occurred, we should close the session
				// after this, session.AcceptStream() will raise the error
				// which specific in session.CloseWithError()
				c.conn.Close()
				// c.Session.Close()
				break
			}
			// frame type
			frameType := f.Type()
			logger.Debug("[parsed]", "type", frameType.String(), "frame", logger.BytesString(f.Encode()))
			switch frameType {
			case frame.TagOfPongFrame:
				c.conn.Heartbeat <- true

			case frame.TagOfAcceptedFrame:
				// create stream
				if c.conn.Type == core.ConnTypeSource || c.conn.Type == core.ConnTypeZipperSender {
					stream, err := c.Session.CreateStream(context.Background())
					if err != nil {
						logger.Error("[client] session.CreateStream Error:", "err", err)
						break
					}

					c.Stream = core.NewFrameStream(stream)
				}
				accepted <- true

			case frame.TagOfRejectedFrame:
				if c.conn.Type == core.ConnTypeStreamFunction {
					logger.Warn("[client] the connection was rejected by zipper, please check if the function name matches the one in zipper config.")
				} else {
					logger.Warn("[client] the connection was rejected by zipper.")
				}
				c.Close()
				c.isRejected = true
				break

			default:
				logger.Debug("[client] unknown signal.", "frame", logger.BytesString(f.Encode()))
			}
		}
	}()
}

// Ping sends the PingFrame to YoMo-Zipper in every 3s.
func (c *Impl) ping() {
	go func(c *Impl) {
		t := time.NewTicker(3 * time.Second)
		for {
			select {
			case <-t.C:
				err := c.conn.SendSignal(frame.NewPingFrame())
				logger.Info("Send Ping to zipper.")
				if err != nil {
					if err.Error() == quic.ErrConnectionClosed {
						logger.Print("[client] ❌ the zipper was offline.")
					} else {
						// other errors.
						logger.Error("[client] ❌ sent Ping to zipper failed.", "err", err)
					}

					t.Stop()
					break
				}
			}
		}
	}(c)
}

// Retry the connection between client and server.
func (c *Impl) Retry() {
	for {
		logger.Debug("[client] retry to connect the YoMo-Zipper...", "addr", getServerAddr(c.serverIP, c.serverPort))
		_, err := c.BaseConnect(c.serverIP, c.serverPort)
		if err == nil {
			break
		}

		time.Sleep(3 * time.Second)
	}
}

// RetryWithCount the connection with a certain count.
func (c *Impl) RetryWithCount(count int) bool {
	for i := 0; i < count; i++ {
		logger.Debug("[client] retry to connect the YoMo-Zipper with count...", "addr", getServerAddr(c.serverIP, c.serverPort), "count", count)
		_, err := c.BaseConnect(c.serverIP, c.serverPort)
		if err == nil {
			return true
		}

		time.Sleep(3 * time.Second)
	}
	return false
}

// Close the client.
func (c *Impl) Close() error {
	logger.Debug("[client] close the connection to YoMo-Zipper.")
	if c.Session != nil {
		err := c.Session.Close()
		if err != nil {
			return err
		}
	}

	err := c.conn.Close()
	c.conn.Heartbeat = make(chan bool)
	c.conn.Signal = nil
	return err
}

// EnableDebug enables the development model for logging.
func (c *Impl) EnableDebug() {
	logger.EnableDebug()
}

func getServerAddr(ip string, port int) string {
	return fmt.Sprintf("%s:%d", ip, port)
}
