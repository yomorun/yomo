package zipper

import (
	"errors"
	"net"

	"github.com/yomorun/yomo/core/quic"
	"github.com/yomorun/yomo/internal/core"
	"github.com/yomorun/yomo/internal/frame"
	"github.com/yomorun/yomo/logger"
)

// Conn represents the YoMo Zipper connection.
type Conn struct {
	conn              *quic.Conn
	Session           quic.Session
	onClosed          func()      // onClosed is the callback when the connection is closed.
	isNewAppAvailable func() bool // indicates whether the server receives a new app.
}

// NewConn inits a new YoMo Zipper connection.
func NewConn(sess quic.Session, st quic.Stream, conf *WorkflowConfig) *Conn {
	logger.Debug("[zipper] inits a new connection.")
	c := &Conn{
		conn: quic.NewConn("", core.ConnTypeNone),
	}

	c.Session = sess
	c.conn.Signal = core.NewFrameStream(st)
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

// handleSignal handles the logic when receiving signal from client.
func (c *Conn) handleSignal(conf *WorkflowConfig) {
	go func() {
		for {
			logger.Info("💚 waiting read next..")
			f, err := c.conn.Signal.ReadFrame()
			if err != nil {
				logger.Error("[ERR] on [ParseFrame]", "err", err)
				if errors.Is(err, net.ErrClosed) {
					// if client close the connection, net.ErrClosed will be raise
					// by quic-go IdleTimeoutError after connection's KeepAlive config.
					logger.Error("[ERR] on [ParseFrame]", "err", net.ErrClosed)
				}
				// any error occurred, we should close the session
				// after this, session.AcceptStream() will raise the error
				// which specific in session.CloseWithError()
				c.conn.Close()
				c.Session.CloseWithError(0xCC, err.Error())
				break
			}

			frameType := f.Type()
			switch frameType {
			case frame.TagOfHandshakeFrame:
				// get negotiation payload
				payload, ok := f.(*frame.HandshakeFrame)
				if !ok {
					// logger.Error("[ERR] HandshakeFrame","err",errors.New(""))
					// TODO
					return
				}

				c.conn.Name = payload.Name
				c.conn.Type = c.getConnType(payload, conf)
				if c.conn.Type == core.ConnTypeNone {
					logger.Printf("The %s name %s is mismatched with the name of Stream Function in zipper config.", payload.ClientType, payload.Name)
					c.conn.SendSignal(frame.NewRejectedFrame())
					continue
				}
				logger.Printf("Receive App %s, type: %s", c.conn.Name, c.conn.Type)

				if c.conn.Type == core.ConnTypeStreamFunction {
					// clear local cache when zipper has a new stream-fn connection.
					clearStreamFuncCache(c.conn.Name)

					// add new connection to channcel
					if ch, ok := newStreamFuncSessionCache.Load(c.conn.Name); ok {
						ch.(chan quic.Session) <- c.Session
					} else {
						ch := make(chan quic.Session, 5)
						ch <- c.Session
						newStreamFuncSessionCache.Store(c.conn.Name, ch)
					}
				}

				c.conn.SendSignal(frame.NewAcceptedFrame())
				c.conn.Healthcheck()

			case frame.TagOfPingFrame:
				c.conn.Heartbeat <- true
			}
		}
	}()
}

func (c *Conn) getConnType(payload *frame.HandshakeFrame, conf *WorkflowConfig) core.ConnectionType {
	clientType := core.ConnectionType(payload.ClientType)
	switch clientType {
	case core.ConnTypeStreamFunction:
		// check if the app name is in functions
		if len(conf.Functions) == 0 {
			return core.ConnTypeNone
		}

		for _, app := range conf.Functions {
			if app.Name == payload.Name {
				return core.ConnTypeStreamFunction
			}
		}
		// name is not found
		return core.ConnTypeNone
	default:
		return clientType
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
