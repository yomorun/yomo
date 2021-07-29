package quic

import (
	"errors"
	"time"

	"github.com/yomorun/yomo/internal/decoder"
	"github.com/yomorun/yomo/internal/framing"
)

const (
	ConnTypeSource         string = "source"          // ConnTypeSource is the connection type "source".
	ConnTypeStreamFunction string = "stream-function" // ConnTypeSource is the connection type "stream-function".
	ConnTypeZipperSender   string = "server-sender"   // ConnTypeSource is the connection type "server-sender".

	ErrConnectionClosed string = "Application error 0x0" // the error message when the connection was closed

	HeartbeatTimeOut = 5 * time.Second // HeartbeatTimeOut is the duration when the heartbeat will be time-out.
)

// Conn represents the QUIC connection.
type Conn struct {
	Signal              decoder.ReadWriter // Signal is the specified stream to receive the signal.
	Stream              decoder.ReadWriter // Stream is the stream to receive actual data.
	Type                string             // Type is the type of connection. Possible value: source, stream-function, server-sender
	Name                string             // Name is the name of connection.
	Heartbeat           chan bool          // Heartbeat is the channel to receive heartbeat.
	IsClosed            bool               // IsClosed indicates whether the connection is closed.
	Ready               bool               // Ready indicates whether the connection is ready.
	OnClosed            func() error       // OnClosed is the callback when the connection is closed.
	OnHeartbeatReceived func()             // OnHeartbeatReceived is the callback when the heartbeat is received.
	OnHeartbeatExpired  func()             // OnHeartbeatExpired is the callback when the heartbeat is expired.
}

// NewConn inits a new QUIC connection.
func NewConn(name string, connType string) *Conn {
	return &Conn{
		Name:      name,
		Type:      connType,
		Heartbeat: make(chan bool),
		IsClosed:  false,
		Ready:     true,
	}
}

// SendSignal sends the signal to client.
func (c *Conn) SendSignal(f framing.Frame) error {
	if c.Signal == nil {
		return errors.New("Signal is nil")
	}

	err := c.Signal.Write(f)
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
				} else {
					// didn't set the custom callback function, will break the loop and close the connection.
					break loop
				}
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
