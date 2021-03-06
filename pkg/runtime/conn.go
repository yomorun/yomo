package runtime

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/yomorun/yomo/pkg/client"
	"github.com/yomorun/yomo/pkg/logger"
	"github.com/yomorun/yomo/pkg/quic"
)

// ServerConn represents the YoMo Server connection.
type ServerConn struct {
	conn              *quic.QuicConn
	Session           quic.Session
	onClosed          func()      // onClosed is the callback when the connection is closed.
	isNewAppAvailable func() bool // indicates whether the server receives a new app.
}

// NewServerConn inits a new YoMo Server connection.
func NewServerConn(sess quic.Session, st quic.Stream, conf *WorkflowConfig) *ServerConn {
	c := &ServerConn{
		conn:    quic.NewQuicConn("", ""),
		Session: sess,
	}

	c.conn.Signal = st
	c.handleSignal(conf)
	c.conn.OnClosed = c.Close

	return c
}

// SendSinkFlowSignal sends the signal Flow/Sink to client.
func (c *ServerConn) SendSinkFlowSignal() error {
	if c.conn.Ready {
		c.conn.Ready = false
		return c.conn.SendSignal(quic.SignalFlowSink)
	}
	return nil
}

// handleSignal handles the logic when receiving signal from client.
func (c *ServerConn) handleSignal(conf *WorkflowConfig) {
	isInit := true
	go func() {
		for {
			buf := make([]byte, 1024)
			n, err := c.conn.Signal.Read(buf)

			if err != nil {
				break
			}
			value := buf[:n]

			if isInit {
				// get negotiation payload
				var payload client.NegotiationPayload
				err := json.Unmarshal(value, &payload)
				if err != nil {
					logger.Error("❌ Zipper inits the connection failed.", "err", err)
					return
				}

				c.conn.Name = payload.AppName
				c.conn.Type = c.getConnType(payload, conf)
				logger.Printf("Receive App %s, type: %s", c.conn.Name, c.conn.Type)
				isInit = false

				c.conn.SendSignal(quic.SignalAccepted)
				c.conn.Healthcheck()
				c.Beat()

				continue
			}

			// receive heatbeat from client.
			if bytes.Equal(value, quic.SignalHeartbeat) {
				c.conn.Heartbeat <- value[0]
			}
		}
	}()
}

func (c *ServerConn) getConnType(payload client.NegotiationPayload, conf *WorkflowConfig) string {
	switch payload.ClientType {
	case quic.ConnTypeServerless:
		// check if the app name is in flows
		for _, app := range conf.Flows {
			if app.Name == payload.AppName {
				return quic.ConnTypeFlow
			}
		}
		// check if the app name is in sinks
		for _, app := range conf.Sinks {
			if app.Name == payload.AppName {
				return quic.ConnTypeSink
			}
		}
	}

	return payload.ClientType
}

// Beat sends the heartbeat to clients in every 200ms.
func (c *ServerConn) Beat() {
	go func(c *ServerConn) {
		t := time.NewTicker(200 * time.Millisecond)
		for {
			select {
			case <-t.C:
				err := c.conn.SendSignal(quic.SignalHeartbeat)
				if err != nil {
					if err.Error() == "Application error 0x0" {
						// when the app reconnected immediately before the heartbeat expiration time (5s), it shoudn't print the outdated error message.
						// only print the message when there is not any new available app with the same name and type.
						if c.isNewAppAvailable == nil || !c.isNewAppAvailable() {
							logger.Printf("❌ The app %s is disconnected.", c.conn.Name)
						}
					} else {
						logger.Error("❌ Server sent SignalHeartbeat to app failed.", "name", c.conn.Name, "err", err)
					}

					t.Stop()
					break
				}
			}
		}
	}(c)
}

// Close the QUIC connection.
func (c *ServerConn) Close() error {
	err := c.Session.CloseWithError(0, "")

	if c.onClosed != nil {
		c.onClosed()
	}

	return err
}
