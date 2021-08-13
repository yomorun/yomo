package client

import (
	"context"
	// "encoding/json"
	"fmt"
	"time"

	// "github.com/google/martian/log"
	"github.com/yomorun/yomo/core/quic"
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
	Stream     *quic.FrameStream // Stream is the stream to receive actual data from source.
	isRejected bool
	QuicStream quic.Stream
}

// New creates a new client.
func New(appName string, clientType string) *Impl {
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
		}
		c.Session = nil

		// reset Stream to nil.
		if c.Stream != nil {
			c.Stream.Close()
			c.Stream = nil
		}

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

	// create stream
	stream, err := client.CreateStreamSync(context.Background())
	if err != nil {
		logger.Error("[client] CreateStream Error:", "err", err)
		return c, err
	}

	// // Send handshake frame
	// handshake := frame.NewHandshakeFrame(c.conn.Name, c.conn.Type)
	// buf := handshake.Encode()
	// logger.Printf(fmt.Sprintf("> [%# x]", buf))
	// stream.Write(buf)

	// set session and signal
	c.Session = client
	c.conn.Signal = quic.NewFrameStream(stream)

	// send handshake to YoMo-Zipper
	err = c.conn.SendSignal(frame.NewHandshakeFrame(c.conn.Name, c.conn.Type))

	if err != nil {
		logger.Error("[client] SendSignal Error:", "err", err)
		return c, err
	}

	accepted := make(chan bool)

	c.handleSignal(accepted)
	c.conn.Healthcheck()

	// waiting when the connection is accepted.
	<-accepted

	if c.isRejected {
		logger.Printf("❌ The connection to YoMo-Zipper %s was rejected.", addr)
	} else {
		logger.Printf("✅ Connected to YoMo-Zipper %s.", addr)
	}

	// ping zipper
	c.ping()

	c.QuicStream = stream

	return c, nil
}

// handleSignal handles the logic when receiving signal from server.
func (c *Impl) handleSignal(accepted chan bool) {
	go func() {
		defer close(accepted)
		for {
			signal := c.conn.Signal
			if signal == nil {
				logger.Error("[client] Signal is nil.")
				break
			}

			f, err := signal.Read()
			if err != nil {
				if err.Error() == quic.ErrConnectionClosed {
					logger.Error("[client] Read the signal failed, the zipper was disconnected.")
					break
				} else {
					logger.Error("[client] Read the signal failed.", "err", err)
					continue
				}
			}

			switch f.Type() {
			case frame.TagOfPongFrame:
				c.conn.ReceivedPingPong <- true

			case frame.TagOfAcceptedFrame:
				// create stream
				if c.conn.Type == quic.ConnTypeSource || c.conn.Type == quic.ConnTypeZipperSender {
					stream, err := c.Session.CreateStream(context.Background())
					if err != nil {
						logger.Error("[client] session.CreateStream Error:", "err", err)
						break
					}

					c.Stream = quic.NewFrameStream(stream)
				}
				accepted <- true

			case frame.TagOfRejectedFrame:
				if c.conn.Type == quic.ConnTypeStreamFunction {
					logger.Warn("[client] the connection was rejected by zipper, please check if the function name matches the one in zipper config.")
				} else {
					logger.Warn("[client] the connection was rejected by zipper.")
				}
				c.Close()
				c.isRejected = true
				break

			default:
				logger.Debug("[client] unknown signal.", "type", f.Type())
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
				logger.Debug("Send Ping to zipper.")
				if err != nil {
					if err.Error() == quic.ErrConnectionClosed {
						// when the app reconnected immediately before the heartbeat expiration time (5s), it shoudn't print the outdated error message.
						// only print the message when there is not any new available app with the same name and type.
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
	c.conn.ReceivedPingPong = make(chan bool)
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
