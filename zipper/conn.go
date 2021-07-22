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
	conn              *quic.QuicConn
	Session           quic.Session
	onClosed          func()      // onClosed is the callback when the connection is closed.
	onGotAppType      func()      // onGotAppType is the callback when the YoMo-Zipper got app type from client's signal.
	isNewAppAvailable func() bool // indicates whether the server receives a new app.
}

// NewConn inits a new YoMo Zipper connection.
func NewConn(sess quic.Session, st quic.Stream, conf *WorkflowConfig) *Conn {
	c := &Conn{
		conn:    quic.NewQuicConn("", ""),
		Session: sess,
	}

	c.conn.Signal = st
	c.handleSignal(conf)
	c.conn.OnClosed = c.Close

	return c
}

// SendSignalCreateStream sends the signal `function` to client.
func (c *Conn) SendSignalCreateStream() error {
	if c.conn.Ready {
		c.conn.Ready = false
		return c.conn.SendSignal(framing.NewCreateStreamFrame().Bytes())
	}
	return nil
}

const mismatchedFuncName = "mismatched function name"

// handleSignal handles the logic when receiving signal from client.
func (c *Conn) handleSignal(conf *WorkflowConfig) {
	go func() {
		fd := decoder.NewFrameDecoder(c.conn.Signal)
		for {
			buf, err := fd.Read(true)
			if err != nil {
				logger.Error("[zipper conn] FrameDecoder read failed:", "err", err)
				break
			}

			f, err := framing.FromRawBytes(buf)
			if err != nil {
				logger.Error("[zipper conn] framing.FromRawBytes failed:", "err", err)
				break
			}

			switch f.Type() {
			case framing.FrameTypeHandshake:
				// get negotiation payload
				var payload client.NegotiationPayload
				err := json.Unmarshal(f.Data(), &payload)
				if err != nil {
					logger.Error("❌ YoMo-Zipper inits the connection failed.", "err", err)
					return
				}

				c.conn.Name = payload.AppName
				c.conn.Type = c.getConnType(payload, conf)
				if c.conn.Type == mismatchedFuncName {
					logger.Printf("The %s name %s is mismatched with the one in zipper config.", payload.ClientType, payload.AppName)
					c.conn.SendSignal(framing.NewRejectedFrame().Bytes())
					continue
				}
				logger.Printf("Receive App %s, type: %s", c.conn.Name, c.conn.Type)

				if c.onGotAppType != nil {
					c.onGotAppType()
				}

				c.conn.SendSignal(framing.NewAcceptedFrame().Bytes())
				c.conn.Healthcheck()
				c.Beat()

				// create stream when the connetion is initialized.
				if c.conn.Type == quic.ConnTypeStreamFunction {
					c.createStream()
				}

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
				err := c.conn.SendSignal(framing.NewHeartbeatFrame().Bytes())
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

// createStream sends the singal to create a stream for receiving data.
func (c *Conn) createStream() {
	go func(c *Conn) {
		t := time.NewTicker(200 * time.Millisecond)
		for {
			select {
			case <-t.C:
				// skip if the stream was created or the conn was closed.
				if c.conn.Stream != nil || c.conn.IsClosed {
					t.Stop()
					break
				}

				// send the signal to create stream.
				err := c.SendSignalCreateStream()
				if err != nil {
					logger.Error("❌ Server sent SignalFunction to app failed.", "name", c.conn.Name, "err", err)
				} else {
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
