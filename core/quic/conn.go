package quic

import (
	"errors"
	"io"
	"time"
)

const (
	ConnTypeSource         string = "source"
	ConnTypeStreamFunction string = "stream-function"
	ConnTypeZipperSender   string = "server-sender"

	ErrConnectionClosed string = "Application error 0x0" // the error message when the connection was closed

	HeartbeatTimeOut = 5 * time.Second // HeartbeatTimeOut is the duration when the heartbeat will be time-out.
)

// QuicConn represents the QUIC connection.
type QuicConn struct {
	Signal              io.ReadWriter // Signal is the specified stream to receive the signal.
	Stream              io.ReadWriter // Stream is the stream to receive actual data.
	Type                string        // Type is the type of connection. Possible value: source, stream-function, server-sender
	Name                string        // Name is the name of connection.
	Heartbeat           chan bool     // Heartbeat is the channel to receive heartbeat.
	IsClosed            bool          // IsClosed indicates whether the connection is closed.
	Ready               bool          // Ready indicates whether the connection is ready.
	OnClosed            func() error  // OnClosed is the callback when the connection is closed.
	OnHeartbeatReceived func()        // OnHeartbeatReceived is the callback when the heartbeat is received.
	OnHeartbeatExpired  func()        // OnHeartbeatExpired is the callback when the heartbeat is expired.
}

// NewQuicConn inits a new QUIC connection.
func NewQuicConn(name string, connType string) *QuicConn {
	return &QuicConn{
		Name:      name,
		Type:      connType,
		Heartbeat: make(chan bool),
		IsClosed:  false,
		Ready:     true,
	}
}

// SendSignal sends the signal to client.
func (c *QuicConn) SendSignal(b []byte) error {
	if c.Signal == nil {
		return errors.New("Signal is nil")
	}

	_, err := c.Signal.Write(b)
	return err
}

// Healthcheck checks if peer is online by heartbeat.
func (c *QuicConn) Healthcheck() {
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
func (c *QuicConn) Close() error {
	c.IsClosed = true
	c.Ready = true

	if c.OnClosed != nil {
		return c.OnClosed()
	}
	return nil
}
