package quic

import (
	"errors"
	"io"
	"time"
)

const (
	ConnTypeSource          string = "source"
	ConnTypeStreamFunction  string = "stream-function"
	ConnTypeOutputConnector string = "output-connector"
	ConnTypeZipperSender    string = "zipper-sender"

	ErrConnectionClosed string = "Application error 0x0" // the error message when the connection was closed
)

var (
	// SignalHeartbeat represents the signal of Heartbeat.
	SignalHeartbeat = []byte{0}

	// SignalAccepted represents the signal of Accpeted.
	SignalAccepted = []byte{1}

	// SignalFunction represents the signal for Stream Function and Output Connector.
	SignalFunction = []byte{0, 0}
)

// QuicConn represents the QUIC connection.
type QuicConn struct {
	Signal              io.ReadWriter
	Stream              io.ReadWriter
	Type                string
	Name                string
	Heartbeat           chan byte
	IsClosed            bool
	Ready               bool
	OnClosed            func() error // OnClosed is the callback when the connection is closed.
	OnHeartbeatReceived func()       // OnHeartbeatReceived is the callback when the heartbeat is received.
	OnHeartbeatExpired  func()       // OnHeartbeatExpired is the callback when the heartbeat is expired.
}

// NewQuicConn inits a new QUIC connection.
func NewQuicConn(name string, connType string) *QuicConn {
	return &QuicConn{
		Name:      name,
		Type:      connType,
		Heartbeat: make(chan byte),
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

// Healthcheck checks if receiving the heartbeat from peer.
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

			case <-time.After(5 * time.Second):
				// didn't receive the heartbeat after 5s, call the callback function when expired.
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

// Close the QUIC connections.
func (c *QuicConn) Close() error {
	c.IsClosed = true
	c.Ready = true

	if c.OnClosed != nil {
		return c.OnClosed()
	}
	return nil
}
