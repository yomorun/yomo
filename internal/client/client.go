package client

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/yomorun/yomo/core/quic"
	"github.com/yomorun/yomo/internal/decoder"
	"github.com/yomorun/yomo/internal/framing"
	"github.com/yomorun/yomo/logger"
)

// streamPoolCount is the number of streams in pool.
const streamPoolCount = 10

// NegotiationPayload represents the payload for negotiation.
type NegotiationPayload struct {
	AppName    string `json:"app_name"`    // AppName is the name of client.
	ClientType string `json:"client_type"` // ClientType is the type of client.
}

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
	Stream     decoder.ReadWriter // Stream is the stream to receive actual data from source.
	once       *sync.Once
	isRejected bool
}

// New creates a new client.
func New(appName string, clientType string) *Impl {
	c := &Impl{
		conn: quic.NewConn(appName, clientType),
		once: new(sync.Once),
	}

	c.conn.OnHeartbeatReceived = func() {
		// when the client received the heartbeat from server, send it back back to server.
		c.conn.SendSignal(framing.NewHeartbeatFrame())
	}

	c.conn.OnHeartbeatExpired = func() {
		if c.isRejected {
			// the connection was rejected by YoMo-Zipper, don't need to re-connect.
			return
		}

		// retry the connection.
		c.once.Do(func() {
			logger.Debug("[client] heartbeat to YoMo-Zipper was expired, client will reconnect to YoMo-Zipper.", "addr", getServerAddr(c.serverIP, c.serverPort))

			// reset session to nil.
			if c.Session != nil {
				c.Session.Close()
			}
			c.Session = nil

			// reset Stream to nil.
			c.Stream = nil

			// reconnect when the heartbeat is expired.
			c.BaseConnect(c.serverIP, c.serverPort)

			// reset the sync.Once after 5s.
			time.AfterFunc(5*time.Second, func() {
				c.once = new(sync.Once)
			})
		})
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
	stream, err := client.CreateStream(context.Background())
	if err != nil {
		logger.Error("[client] CreateStream Error:", "err", err)
		return c, err
	}

	// set session and signal
	c.Session = client
	c.conn.Signal = decoder.NewReadWriter(stream)

	// send negotiation payload to YoMo-Zipper
	payload := NegotiationPayload{
		AppName:    c.conn.Name,
		ClientType: c.conn.Type,
	}
	buf, _ := json.Marshal(payload)
	err = c.conn.SendSignal(framing.NewHandshakeFrame(buf))

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
	return c, nil
}

// handleSignal handles the logic when receiving signal from server.
func (c *Impl) handleSignal(accepted chan bool) {
	go func() {
		defer close(accepted)
		frameCh := c.conn.Signal.Read()
		for frame := range frameCh {
			switch frame.Type() {
			case framing.FrameTypeHeartbeat:
				c.conn.Heartbeat <- true

			case framing.FrameTypeAccepted:
				// create stream
				if c.conn.Type == quic.ConnTypeSource || c.conn.Type == quic.ConnTypeZipperSender {
					stream, err := c.Session.CreateStream(context.Background())
					if err != nil {
						logger.Error("[client] session.CreateStream Error:", "err", err)
						break
					}

					c.Stream = decoder.NewReadWriter(stream)
				}
				accepted <- true

			case framing.FrameTypeRejected:
				if c.conn.Type == quic.ConnTypeStreamFunction {
					logger.Warn("[client] the connection was rejected by zipper, please check if the function name matches the one in zipper config.")
				} else {
					logger.Warn("[client] the connection was rejected by zipper.")
				}
				c.Close()
				c.isRejected = true
				break

			default:
				logger.Debug("[client] unknown signal.", "frame", logger.BytesString(frame.Bytes()))
			}
		}
	}()
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
