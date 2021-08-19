package quic

import (
	"time"

	"github.com/yomorun/yomo/internal/frame"
)

const (
	// ConnTypeSource is the connection type "source".
	ConnTypeSource string = "source"
	// ConnTypeSource is the connection type "stream-function".
	ConnTypeStreamFunction string = "stream-function"
	// ConnTypeSource is the connection type "server-sender".
	ConnTypeZipperSender string = "server-sender"
	// the error message when the connection was closed
	ErrConnectionClosed string = "Application error 0x0"
	// HeartbeatTimeOut is the duration when the heartbeat will be time-out.
	HeartbeatTimeOut = 5 * time.Second
)

// Conn represents the QUIC connection.
type Conn struct {
	Signal              *FrameStream // Signal is the specified stream to receive the signal from peer.
	Type                string       // Type is the type of connection. Possible value: source, stream-function, server-sender
	Name                string       // Name is the name of connection.
	ReceivedPingPong    chan bool    // Heartbeat is the channel to receive  ping/pingheartbeat.
	IsClosed            bool         // IsClosed indicates whether the connection is closed.
	Ready               bool         // Ready indicates whether the connection is ready.
	OnClosed            func() error // OnClosed is the callback when the connection is closed.
	OnHeartbeatReceived func()       // OnHeartbeatReceived is the callback when the heartbeat is received.
	OnHeartbeatExpired  func()       // OnHeartbeatExpired is the callback when the heartbeat is expired.
}

// NewConn inits a new QUIC connection.
func NewConn(name string, connType string) *Conn {
	return &Conn{
		Name:             name,
		Type:             connType,
		ReceivedPingPong: make(chan bool),
		IsClosed:         false,
		Ready:            true,
	}
}

// SendSignal sends the signal to client.
func (c *Conn) SendSignal(f frame.Frame) error {
	_, err := c.Signal.Write(f)
	return err
}

// Healthcheck checks if peer is online by Ping/Pong heartbeat.
func (c *Conn) Healthcheck() {
	go func() {
		// receive heartbeat
		defer c.Close()
	loop:
		for {
			select {
			case _, ok := <-c.ReceivedPingPong:
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

				// break the loop and close the connection.
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
