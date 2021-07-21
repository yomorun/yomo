package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/yomorun/yomo/core/quic"
	"github.com/yomorun/yomo/internal/framing"
	"github.com/yomorun/yomo/logger"
)

// NegotiationPayload represents the payload for negotiation.
type NegotiationPayload struct {
	AppName    string `json:"app_name"`    // AppName is the name of client.
	ClientType string `json:"client_type"` // ClientType is the type of client.
}

// Client is the interface for common functions of YoMo client.
type Client interface {
	io.Writer

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
	conn       *quic.QuicConn
	serverIP   string
	serverPort int
	Readers    chan io.Reader // Readers are the reader to receive the data from YoMo Zipper.
	Writer     io.Writer      // Writer is the stream to send the data to YoMo Zipper.
	session    quic.Client
	once       *sync.Once
}

// New creates a new client.
func New(appName string, clientType string) *Impl {
	c := &Impl{
		conn:    quic.NewQuicConn(appName, clientType),
		Readers: make(chan io.Reader, 1),
		once:    new(sync.Once),
	}

	c.conn.OnHeartbeatReceived = func() {
		// when the client received the heartbeat from server, send it back back to server (ping/pong).
		c.conn.SendSignal(quic.SignalHeartbeat)
	}

	c.conn.OnHeartbeatExpired = func() {
		c.once.Do(func() {
			logger.Debug("[client] heartbeat to YoMo-Zipper was expired, client will reconnect to YoMo-Zipper.", "addr", getServerAddr(c.serverIP, c.serverPort))

			// reset stream to nil.
			c.conn.Stream = nil

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
	quic_cli, err := quic.NewClient(addr)
	if err != nil {
		logger.Error("[client] NewClient Error:", "err", err)
		return c, err
	}

	// create stream
	quic_stream, err := quic_cli.CreateStream(context.Background())
	if err != nil {
		logger.Error("[client] CreateStream Error:", "err", err)
		return c, err
	}

	// set session and signal
	c.session = quic_cli
	c.conn.Signal = quic_stream

	// send negotiation payload to YoMo-Zipper
	payload := NegotiationPayload{
		AppName:    c.conn.Name,
		ClientType: c.conn.Type,
	}
	buf, _ := json.Marshal(payload)
	err = c.conn.SendSignal(buf)

	if err != nil {
		logger.Error("[client] SendSignal Error:", "err", err)
		return c, err
	}

	accepted := make(chan bool)

	c.handleSignal(accepted)
	c.conn.Healthcheck()

	// waiting when the connection is accepted.
	<-accepted
	logger.Printf("✅ Connected to YoMo-Zipper %s.", addr)
	return c, nil
}

// handleSignal handles the logic when receiving signal from server.
func (c *Impl) handleSignal(accepted chan bool) {
	go func() {
		defer close(accepted)
		for {
			buf := make([]byte, 2)
			n, err := c.conn.Signal.Read(buf)
			if err != nil {
				break
			}
			value := buf[:n]
			if bytes.Equal(value, quic.SignalHeartbeat) {
				// heartbeat
				c.conn.Heartbeat <- buf[0]
			} else if bytes.Equal(value, quic.SignalAccepted) {
				// accepted
				if c.conn.Type == quic.ConnTypeSource || c.conn.Type == quic.ConnTypeZipperSender {
					// create stream for source.
					stream, err := c.session.CreateStream(context.Background())
					if err != nil {
						logger.Error("[client] session.CreateStream Error:", "err", err)
						break
					}
					c.conn.Stream = stream
				}
				accepted <- true
			} else if bytes.Equal(value, quic.SignalFunction) {
				// create stream for Stream Function/Output Connector.
				stream, err := c.session.CreateStream(context.Background())

				if err != nil {
					logger.Error("[client] session.CreateStream Error:", "err", err)
					break
				}

				c.Readers <- stream
				c.Writer = stream
				stream.Write(quic.SignalHeartbeat)
			} else {
				logger.Debug("[client] unknown signal.", "value", logger.BytesString(value))
			}
		}
	}()
}

// Write the data to downstream.
func (c *Impl) Write(data []byte) (int, error) {
	if c.conn.Stream != nil {
		// wrap data with framing.
		f := framing.NewPayloadFrame(data)
		return c.conn.Stream.Write(f.Bytes())
	} else {
		return 0, errors.New("[client] conn.Stream is nil.")
	}
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
	err := c.session.Close()
	c.conn.Heartbeat = make(chan byte)
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
