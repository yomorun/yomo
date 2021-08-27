package quic

import (
	"errors"
	"time"

	"github.com/yomorun/yomo/internal/core"
	"github.com/yomorun/yomo/internal/frame"
)

const (
	// ErrConnectionClosed is the error message when the connection was closed.
	ErrConnectionClosed string = "Application error 0x0"
	// HeartbeatTimeOut is the duration when the heartbeat will be time-out.
	HeartbeatTimeOut = 5 * time.Second
)

// Conn represents the QUIC connection.
type Conn struct {
	// Signal is the specified stream to receive the signal.
	Signal *core.FrameStream
	// Type is the type of connection. Possible value: source, stream-function, server-sender.
	Type core.ConnectionType
	// Name is the name of connection.
	Name string
	// Heartbeat is the channel to receive heartbeat.
	Heartbeat chan bool
	// IsClosed indicates whether the connection is closed.
	IsClosed bool
	// Ready indicates whether the connection is ready.
	Ready bool
	// OnClosed is the callback when the connection is closed.
	OnClosed func() error
	// OnHeartbeatReceived is the callback when the heartbeat is received.
	OnHeartbeatReceived func()
	// OnHeartbeatExpired is the callback when the heartbeat is expired.
	OnHeartbeatExpired func()
}

// NewConn inits a new QUIC connection.
func NewConn(name string, connType core.ConnectionType) *Conn {
	return &Conn{
		Name:      name,
		Type:      connType,
		Heartbeat: make(chan bool),
		IsClosed:  false,
		Ready:     true,
	}
}

// SendSignal sends the signal to client.
func (c *Conn) SendSignal(f frame.Frame) error {
	if c.Signal == nil {
		return errors.New("Signal is nil")
	}

	_, err := c.Signal.WriteFrame(f)
	return err
}

// Healthcheck checks if peer is online by heartbeat.
func (c *Conn) Healthcheck() {
	go func() {
		// receive heartbeat
		defer c.Close()
	loop:
		for {
			select {
			case _, ok := <-c.Heartbeat:
				if !ok {
					break loop
				}
				if c.OnHeartbeatReceived != nil {
					c.OnHeartbeatReceived()
				}

			case <-time.After(HeartbeatTimeOut):
				// didn't receive the heartbeat after a certain duration, call the callback function when expired.
				if c.OnHeartbeatExpired != nil {
					c.OnHeartbeatExpired()
				}

				break loop
			}
		}
	}()
}

// Close the QUIC connection.
func (c *Conn) Close() error {
	c.IsClosed = true
	c.Ready = true

	if c.OnClosed != nil {
		return c.OnClosed()
	}
	return nil
}
