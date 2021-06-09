package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/yomorun/yomo/pkg/client"
	"github.com/yomorun/yomo/pkg/quic"
)

// Conn represents a YoMo Server connection.
type Conn interface {
	// SendSignal sends the signal to client.
	SendSignal(b []byte) error

	// SendSinkFlowSignal sends the signal Flow/Sink to client.
	SendSinkFlowSignal() error

	// Beat sends the heartbeat to clients and checks if receiving the heartbeat back.
	Beat()

	// Close the connection.
	Close()

	// GetStream gets the current stream in connection.
	GetStream() io.ReadWriter

	// GetStreamType gets the type of current stream.
	GetStreamType() string

	// OnRead reads new stream and calls the callback handler.
	OnRead(st io.ReadWriter, handler func())

	// OnClosed sets the `OnClosed` handler.
	OnClosed(handler func())

	// IsMatched indicates if the connection is matched.
	IsMatched(streamType string, name string) bool
}

// NewConn inits a new YoMo Server connection.
func NewConn(sess quic.Session, st quic.Stream, conf *WorkflowConfig) Conn {
	conn := &quicConn{
		Session:    sess,
		Signal:     st,
		StreamType: "",
		Name:       "",
		Heartbeat:  make(chan byte),
		IsClosed:   false,
		Ready:      true,
	}

	conn.Init(conf)
	return conn
}

const (
	StreamTypeSource       string = "source"
	StreamTypeFlow         string = "flow"
	StreamTypeSink         string = "sink"
	StreamTypeZipperSender string = "zipper-sender"
)

// quicConn represents the QUIC connection.
type quicConn struct {
	Session    quic.Session
	Signal     quic.Stream
	Stream     io.ReadWriter
	StreamType string
	Name       string
	Heartbeat  chan byte
	IsClosed   bool
	Ready      bool
	onClosed   func() // onClosed is the callback when the connection is closed.
}

// SendSignal sends the signal to client.
func (c *quicConn) SendSignal(b []byte) error {
	_, err := c.Signal.Write(b)
	return err
}

// SendSinkFlowSignal sends the signal Flow/Sink to client.
func (c *quicConn) SendSinkFlowSignal() error {
	if c.Ready {
		c.Ready = false
		return c.SendSignal(client.SignalFlowSink)
	}
	return nil
}

// Init the QUIC connection.
func (c *quicConn) Init(conf *WorkflowConfig) {
	isInit := true
	go func() {
		for {
			buf := make([]byte, 1024)
			n, err := c.Signal.Read(buf)

			if err != nil {
				break
			}
			value := buf[:n]

			if isInit {
				// get negotiation payload
				var payload client.NegotiationPayload
				err := json.Unmarshal(value, &payload)
				if err != nil {
					log.Printf("❌ Zipper inits the connection failed: %s", err.Error())
					return
				}

				streamType, err := c.getStreamType(payload, conf)
				if err != nil {
					log.Printf("❌ Zipper get the stream type from the connection failed: %s", err.Error())
					return
				}

				c.Name = payload.AppName
				c.StreamType = streamType
				fmt.Println("Receive App:", c.Name, c.StreamType)
				isInit = false
				c.SendSignal(client.SignalAccepted)
				c.Beat()
				continue
			}

			if bytes.Equal(value, client.SignalHeartbeat) {
				c.Heartbeat <- value[0]
			}
		}
	}()
}

func (c *quicConn) getStreamType(payload client.NegotiationPayload, conf *WorkflowConfig) (string, error) {
	switch payload.ClientType {
	case client.ClientTypeSource:
		return StreamTypeSource, nil
	case client.ClientTypeZipperSender:
		return StreamTypeZipperSender, nil
	case client.ClientTypeServerless:
		// check if the app name is in flows
		for _, app := range conf.Flows {
			if app.Name == payload.AppName {
				return StreamTypeFlow, nil
			}
		}
		// check if the app name is in sinks
		for _, app := range conf.Sinks {
			if app.Name == payload.AppName {
				return StreamTypeSink, nil
			}
		}
	}
	return "", fmt.Errorf("the client %s (type: %s) isn't matched any stream type", payload.AppName, payload.ClientType)
}

// Beat sends the heartbeat to clients and checks if receiving the heartbeat back.
func (c *quicConn) Beat() {
	go func() {
		defer c.Close()
	loop:
		for {
			select {
			case _, ok := <-c.Heartbeat:
				if !ok {
					return
				}

			case <-time.After(5 * time.Second):
				// close the connection if didn't receive the heartbeat after 5s.
				log.Printf("Server didn't receive the heartbeat from the app [%s] after 5s, will close the connection.", c.Name)
				break loop
			}
		}
	}()

	go func() {
		for {
			// send heartbeat in every 200ms.
			time.Sleep(200 * time.Millisecond)
			err := c.SendSignal(client.SignalHeartbeat)
			if err != nil {
				log.Printf("❌ Server sent SignalHeartbeat to app [%s] failed: %s", c.Name, err.Error())
				break
			}
		}
	}()
}

// GetStream gets the current stream in connection.
func (c *quicConn) GetStream() io.ReadWriter {
	return c.Stream
}

// GetStreamType gets the type of current stream.
func (c *quicConn) GetStreamType() string {
	return c.StreamType
}

// Read the new QUIC stream.
func (c *quicConn) OnRead(st io.ReadWriter, handler func()) {
	c.Ready = true
	c.Stream = st

	if handler != nil {
		handler()
	}
}

// Close the QUIC connections.
func (c *quicConn) Close() {
	c.Session.CloseWithError(0, "")
	c.IsClosed = true
	c.Ready = true

	if c.onClosed != nil {
		c.onClosed()
	}
}

// OnClosed sets the `OnClosed` handler.
func (c *quicConn) OnClosed(handler func()) {
	c.onClosed = handler
}

// IsMatched indicates if the connection is matched.
func (c *quicConn) IsMatched(streamType string, name string) bool {
	return c.StreamType == streamType && c.Name == name
}
