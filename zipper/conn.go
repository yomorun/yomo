package zipper

import (
	"errors"
	"net"

	"github.com/yomorun/yomo/core/quic"
	"github.com/yomorun/yomo/internal/core"
	"github.com/yomorun/yomo/internal/frame"
	"github.com/yomorun/yomo/pkg/logger"
)

// Conn represents the YoMo Zipper connection.
type Conn struct {
	// Addr is the peer's address.
	Addr string
	// Conn is a QUIC connection.
	Conn *quic.Conn
	// Session is a QUIC connection.
	Session quic.Session
	// onClosed is the callback when the connection is closed.
	onClosed func()
}

// NewConn inits a new YoMo Zipper connection.
func NewConn(addr string, sess quic.Session, st quic.Stream, conf *WorkflowConfig) *Conn {
	logger.Debug("[zipper] inits a new connection.")
	c := &Conn{
		Conn: quic.NewConn("", core.ClientTypeNone),
	}

	c.Addr = addr
	c.Session = sess
	c.Conn.Signal = core.NewFrameStream(st)
	c.handleSignal(conf)
	c.Conn.OnClosed = c.Close
	c.Conn.OnHeartbeatReceived = func() {
		logger.Debug("Received Ping from client, will send Pong to client.", "name", c.Conn.Name, "addr", c.Addr)
		// when the zipper received Ping from client, send Pong to client.
		c.Conn.SendSignal(frame.NewPongFrame())
	}

	c.Conn.OnHeartbeatExpired = func() {
		logger.Printf("❌ The client %s was offline, addr: %s", c.Conn.Name, c.Addr)
		st.Close()
		c.Conn.Close()
	}

	return c
}

// handleSignal handles the logic when receiving signal from client.
func (c *Conn) handleSignal(conf *WorkflowConfig) {
	go func() {
		for {
			logger.Info("💚 waiting read next..")
			f, err := c.Conn.Signal.ReadFrame()
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
				c.Conn.Close()
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

				c.Conn.Name = payload.Name
				c.Conn.Type = c.getConnType(payload, conf)
				if c.Conn.Type == core.ClientTypeNone {
					logger.Printf("The %s name %s is mismatched with the name of Stream Function in zipper config.", payload.ClientType, payload.Name)
					c.Conn.SendSignal(frame.NewRejectedFrame())
					continue
				}
				logger.Printf("Receive App %s, type: %s, addr: %s", c.Conn.Name, c.Conn.Type, c.Addr)

				if c.Conn.Type == core.ClientTypeStreamFunction {
					// clear local cache when zipper has a new stream-fn connection.
					clearStreamFuncCache(c.Conn.Name)

					// add new connection to channcel
					if ch, ok := newStreamFuncSessionCache.Load(c.Conn.Name); ok {
						ch.(chan quic.Session) <- c.Session
					} else {
						ch := make(chan quic.Session, 5)
						ch <- c.Session
						newStreamFuncSessionCache.Store(c.Conn.Name, ch)
					}
				}

				c.Conn.SendSignal(frame.NewAcceptedFrame())
				c.Conn.Healthcheck()

			case frame.TagOfPingFrame:
				c.Conn.Heartbeat <- true
			}
		}
	}()
}

func (c *Conn) getConnType(payload *frame.HandshakeFrame, conf *WorkflowConfig) core.ClientType {
	clientType := core.ClientType(payload.ClientType)
	switch clientType {
	case core.ClientTypeStreamFunction:
		// check if the app name is in functions
		if len(conf.Functions) == 0 {
			return core.ClientTypeNone
		}

		for _, app := range conf.Functions {
			if app.Name == payload.Name {
				return core.ClientTypeStreamFunction
			}
		}
		// name is not found
		return core.ClientTypeNone
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
