package server

import (
	"github.com/yomorun/yomo/core/quic"
	"github.com/yomorun/yomo/internal/frame"
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
func NewConn(sess quic.Session, st quic.Stream, conf *Config) *Conn {
	logger.Debug("inits a new connection.")
	c := &Conn{
		conn: quic.NewConn("", ""),
	}

	c.Session = sess
	c.conn.Signal = quic.NewFrameStream(st)
	c.handleSignal(conf)
	c.conn.OnClosed = c.Close
	c.conn.OnHeartbeatReceived = func() {
		logger.Debug("Received Ping from client, will send Pong to client.", "name", c.conn.Name)
		// when the zipper received Ping from client, send Pong to client.
		c.conn.SendSignal(frame.NewPongFrame())
	}

	c.conn.OnHeartbeatExpired = func() {
		logger.Printf("❌ The client %s was offline.", c.conn.Name)
		st.Close()
		c.conn.Close()
	}

	return c
}

const mismatchedFuncName = "mismatched function name"

// handleSignal handles the logic when receiving signal from client.
func (c *Conn) handleSignal(conf *Config) {
	go func() {
		for {
			signal := c.conn.Signal
			if signal == nil {
				logger.Error("Signal is nil.")
				break
			}

			f, err := signal.Read()
			if err != nil {
				if err.Error() == quic.ErrConnectionClosed {
					logger.Info("Reand the signail failed, the client was disconneced.", "name", c.conn.Name)
					break
				} else {
					logger.Error("Read the signal failed.", "name", c.conn.Name, "err", err)
					continue
				}
			}

			switch f.Type() {
			case frame.TagOfHandshakeFrame:
				// get negotiation payload
				handshake := f.(*frame.HandshakeFrame)

				c.conn.Name = handshake.Name
				c.conn.Type = c.getConnType(*handshake, conf)
				if c.conn.Type == mismatchedFuncName {
					logger.Printf("The %s name %s is mismatched with the name of Stream Function in zipper config.", handshake.ClientType, handshake.Name)
					c.conn.SendSignal(frame.NewRejectedFrame())
					continue
				}
				logger.Printf("Receive App %s, type: %s", c.conn.Name, c.conn.Type)

				if c.onGotAppType != nil {
					c.onGotAppType()
				}

				c.conn.SendSignal(frame.NewAcceptedFrame())
				c.conn.Healthcheck()

			case frame.TagOfPingFrame:
				logger.Debug("Received Ping from client.", "name", c.conn.Name)
				c.conn.ReceivedPingPong <- true
			}
		}
	}()
}

func (c *Conn) getConnType(handshake frame.HandshakeFrame, conf *Config) string {
	switch handshake.ClientType {
	case quic.ConnTypeStreamFunction:
		// check if the app name is in functions
		if len(conf.Functions) == 0 {
			return quic.ConnTypeStreamFunction
		}

		for _, app := range conf.Functions {
			if app.Name == handshake.Name {
				return quic.ConnTypeStreamFunction
			}
		}
		// name is not found
		return mismatchedFuncName
	default:
		return handshake.ClientType
	}
}

// Close the QUIC connection.
func (c *Conn) Close() error {
	err := c.Session.CloseWithError(0, "")

	if c.onClosed != nil {
		c.onClosed()
	}

	return err
}
