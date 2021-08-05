package server

import (
	"encoding/json"
	"time"

	"github.com/yomorun/yomo/core/quic"
	"github.com/yomorun/yomo/internal/client"
	"github.com/yomorun/yomo/internal/decoder"
	"github.com/yomorun/yomo/internal/framing"
	"github.com/yomorun/yomo/logger"
)

// Conn represents the YoMo Zipper connection.
type Conn struct {
	conn              *quic.Conn
	Session           quic.Session
	onClosed          func()      // onClosed is the callback when the connection is closed.
	onGotAppType      func()      // onGotAppType is the callback when the YoMo-Zipper got app type from client's signal.
	isNewAppAvailable func() bool // indicates whether the server receives a new app.
}

// NewConn inits a new YoMo Zipper connection.
func NewConn(sess quic.Session, st quic.Stream, conf *WorkflowConfig) *Conn {
	logger.Debug("[zipper] inits a new connection.")
	c := &Conn{
		conn:    quic.NewConn("", ""),
		Session: sess,
	}

	c.Session = sess
	c.conn.Signal = decoder.NewReadWriter(st)
	c.handleSignal(conf)
	c.conn.OnClosed = c.Close

	return c
}

// SendSignalCreateStream sends the signal `function` to client.
func (c *Conn) SendSignalCreateStream() error {
	if c.conn.Ready {
		c.conn.Ready = false
		return c.conn.SendSignal(framing.NewCreateStreamFrame())
	}
	return nil
}

const mismatchedFuncName = "mismatched function name"

// handleSignal handles the logic when receiving signal from client.
func (c *Conn) handleSignal(conf *WorkflowConfig) {
	go func() {
		frameCh := c.conn.Signal.Read()
		for frame := range frameCh {
			switch frame.Type() {
			case framing.FrameTypeHandshake:
				// get negotiation payload
				var payload client.NegotiationPayload
				err := json.Unmarshal(frame.Data(), &payload)
				if err != nil {
					logger.Error("❌ YoMo-Zipper inits the connection failed.", "err", err)
					return
				}

				c.conn.Name = payload.AppName
				c.conn.Type = c.getConnType(payload, conf)
				if c.conn.Type == mismatchedFuncName {
					logger.Printf("The %s name %s is mismatched with the name of Stream Function in zipper config.", payload.ClientType, payload.AppName)
					c.conn.SendSignal(framing.NewRejectedFrame())
					continue
				}
				logger.Printf("Receive App %s, type: %s", c.conn.Name, c.conn.Type)

				if c.onGotAppType != nil {
					c.onGotAppType()
				}

				c.conn.SendSignal(framing.NewAcceptedFrame())
				c.conn.Healthcheck()
				c.Beat()

			case framing.FrameTypeHeartbeat:
				c.conn.Heartbeat <- true
			}
		}
	}()
}

func (c *Conn) getConnType(payload client.NegotiationPayload, conf *WorkflowConfig) string {
	switch payload.ClientType {
	case quic.ConnTypeStreamFunction:
		// check if the app name is in functions
		if len(conf.Functions) == 0 {
			return quic.ConnTypeStreamFunction
		}

		for _, app := range conf.Functions {
			if app.Name == payload.AppName {
				return quic.ConnTypeStreamFunction
			}
		}
		// name is not found
		return mismatchedFuncName
	default:
		return payload.ClientType
	}
}

// Beat sends the heartbeat to clients in every 200ms.
func (c *Conn) Beat() {
	go func(c *Conn) {
		t := time.NewTicker(200 * time.Millisecond)
		for {
			select {
			case <-t.C:
				err := c.conn.SendSignal(framing.NewHeartbeatFrame())
				if err != nil {
					if err.Error() == quic.ErrConnectionClosed {
						// when the app reconnected immediately before the heartbeat expiration time (5s), it shoudn't print the outdated error message.
						// only print the message when there is not any new available app with the same name and type.
						if c.isNewAppAvailable == nil || !c.isNewAppAvailable() {
							logger.Printf("❌ The app %s is disconnected.", c.conn.Name)
						}
					} else {
						// other errors.
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
func (c *Conn) Close() error {
	err := c.Session.CloseWithError(0, "")

	if c.onClosed != nil {
		c.onClosed()
	}

	return err
}
