package quic

import (
	"errors"
	"io"
	"time"
)

const (
	ConnTypeSource       string = "source"
	ConnTypeFlow         string = "flow"
	ConnTypeSink         string = "sink"
	ConnTypeServerless   string = "serverless"
	ConnTypeZipperSender string = "zipper-sender"
)

var (
	// SignalHeartbeat represents the signal of Heartbeat.
	SignalHeartbeat = []byte{0}

	// SignalAccepted represents the signal of Accpeted.
	SignalAccepted = []byte{1}

	// SignalFlowSink represents the signal for flow/sink.
	SignalFlowSink = []byte{0, 0}
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
				}
				break loop
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
