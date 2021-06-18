package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/yomorun/yomo/pkg/framing"
	"github.com/yomorun/yomo/pkg/quic"
)

// NegotiationPayload represents the payload for negotiation.
type NegotiationPayload struct {
	AppName    string `json:"app_name"`
	ClientType string `json:"client_type"`
}

type client interface {
	io.Writer

	// Close the client.
	Close() error

	// Retry the connection between client and server.
	Retry()

	// RetryWithCount the connection with a certain count.
	RetryWithCount(count int) bool
}

type clientImpl struct {
	conn       *quic.QuicConn
	zipperIP   string
	zipperPort int
	readers    chan io.Reader
	writer     io.Writer
	session    quic.Client
	once       *sync.Once
}

// newClient creates a new client.
func newClient(appName string, clientType string) *clientImpl {
	c := &clientImpl{
		conn:    quic.NewQuicConn(appName, clientType),
		readers: make(chan io.Reader, 1),
		once:    new(sync.Once),
	}

	c.conn.OnHeartbeatReceived = func() {
		// when the client received the heartbeat from server, send it back back to server (ping/pong).
		c.conn.SendSignal(quic.SignalHeartbeat)
	}

	c.conn.OnHeartbeatExpired = func() {
		c.once.Do(func() {
			// reset stream to nil.
			c.conn.Stream = nil

			// reconnect when the heartbeat is expired.
			c.connect(c.zipperIP, c.zipperPort)

			// reset the sync.Once after 5s.
			time.AfterFunc(5*time.Second, func() {
				c.once = new(sync.Once)
			})
		})
	}

	return c
}

// connect to yomo-zipper.
// TODO: login auth
func (c *clientImpl) connect(ip string, port int) (*clientImpl, error) {
	c.zipperIP = ip
	c.zipperPort = port
	addr := fmt.Sprintf("%s:%d", ip, port)
	log.Println("Connecting to zipper", addr, "...")

	// connect to yomo-zipper
	quic_cli, err := quic.NewClient(addr)
	if err != nil {
		fmt.Println("client [NewClient] Error:", err)
		return c, err
	}

	// create stream
	quic_stream, err := quic_cli.CreateStream(context.Background())
	if err != nil {
		fmt.Println("client [CreateStream] Error:", err)
		return c, err
	}

	// set session and signal
	c.session = quic_cli
	c.conn.Signal = quic_stream

	// send negotiation payload to zipper
	payload := NegotiationPayload{
		AppName:    c.conn.Name,
		ClientType: c.conn.Type,
	}
	buf, _ := json.Marshal(payload)
	err = c.conn.SendSignal(buf)

	if err != nil {
		fmt.Println("client [Write] Error:", err)
		return c, err
	}

	accepted := make(chan bool)

	c.handleSignal(accepted)
	c.conn.Healthcheck()

	// waiting when the connection is accepted.
	<-accepted
	log.Print("✅ Connected to zipper ", addr)
	return c, nil
}

// handleSignal handles the logic when receiving signal from server.
func (c *clientImpl) handleSignal(accepted chan bool) {
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
				// heartbeart
				c.conn.Heartbeat <- buf[0]
			} else if bytes.Equal(value, quic.SignalAccepted) {
				// accepted
				if c.conn.Type == quic.ConnTypeSource || c.conn.Type == quic.ConnTypeZipperSender {
					// create stream for source.
					stream, err := c.session.CreateStream(context.Background())
					if err != nil {
						fmt.Println("client [session.CreateStream] Error:", err)
						break
					}
					c.conn.Stream = stream
				}
				accepted <- true
			} else if bytes.Equal(value, quic.SignalFlowSink) {
				// create stream for flow/sink.
				stream, err := c.session.CreateStream(context.Background())

				if err != nil {
					log.Println(err)
					break
				}

				c.readers <- stream
				c.writer = stream
				stream.Write(quic.SignalHeartbeat)
			}
		}
	}()
}

// Write the data to downstream.
func (c *clientImpl) Write(data []byte) (int, error) {
	if c.conn.Stream != nil {
		// wrap data with framing.
		f := framing.NewPayloadFrame(data)
		return c.conn.Stream.Write(f.Bytes())
	} else {
		return 0, errors.New("not found stream")
	}
}

// Retry the connection between client and server.
func (c *clientImpl) Retry() {
	for {
		_, err := c.connect(c.zipperIP, c.zipperPort)
		if err == nil {
			break
		}

		time.Sleep(time.Second)
	}
}

// RetryWithCount the connection with a certain count.
func (c *clientImpl) RetryWithCount(count int) bool {
	for i := 0; i < count; i++ {
		_, err := c.connect(c.zipperIP, c.zipperPort)
		if err == nil {
			return true
		}

		time.Sleep(time.Second)
	}
	return false
}

// Close the client.
func (c *clientImpl) Close() error {
	err := c.session.Close()
	c.conn.Heartbeat = make(chan byte)
	c.conn.Signal = nil
	return err
}
