package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"reflect"
	"sync"
	"sync/atomic"

	"github.com/lucas-clemente/quic-go"
	"github.com/yomorun/yomo/core/auth"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/store"
	"github.com/yomorun/yomo/pkg/logger"
)

const (
	DefaultListenAddr = "0.0.0.0:9000"
)

type ServerOption func(*ServerOptions)

// type FrameHandler func(store store.Store, stream quic.Stream, session quic.Session, f frame.Frame) error
type FrameHandler func(c *Context) error

// Server is the underlining server of Zipper
type Server struct {
	name string
	// stream             quic.Stream
	state              string
	connector          Connector
	router             Router
	counterOfDataFrame int64
	downstreams        map[string]*Client
	mu                 sync.Mutex
	opts               ServerOptions
	beforeHandlers     []FrameHandler
	afterHandlers      []FrameHandler
}

// NewServer create a Server instance.
func NewServer(name string, opts ...ServerOption) *Server {
	s := &Server{
		name:        name,
		connector:   newConnector(),
		downstreams: make(map[string]*Client),
	}
	s.Init(opts...)
	once.Do(func() {
		s.init()
	})

	return s
}

// Init the options.
func (s *Server) Init(opts ...ServerOption) error {
	for _, o := range opts {
		o(&s.opts)
	}
	// options defaults
	s.initOptions()

	return nil
}

// ListenAndServe starts the server.
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	if addr == "" {
		addr = DefaultListenAddr
	}
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}
	return s.Serve(ctx, conn)
}

// Serve the server with a net.PacketConn.
func (s *Server) Serve(ctx context.Context, conn net.PacketConn) error {
	listener := newListener()
	// listen the address
	err := listener.Listen(conn, s.opts.TLSConfig, s.opts.QuicConfig)
	if err != nil {
		logger.Errorf("%squic.ListenAddr on: %s, err=%v", ServerLogPrefix, listener.Addr(), err)
		return err
	}
	defer listener.Close()
	logger.Printf("%s✅ [%s] Listening on: %s, QUIC: %v, AUTH: %s", ServerLogPrefix, s.name, listener.Addr(), listener.Versions(), s.authNames())

	s.state = ConnStateConnected
	for {
		// create a new session when new yomo-client connected
		sctx, cancel := context.WithCancel(ctx)
		defer cancel()

		session, err := listener.Accept(sctx)
		if err != nil {
			logger.Errorf("%screate session error: %v", ServerLogPrefix, err)
			return err
		}

		logger.Infof("%s❤️1/ new connection: %s", ServerLogPrefix, session.RemoteAddr().String())

		go func(ctx context.Context, sess quic.Session) {
			var c *Context
			for {
				logger.Infof("%s❤️2/ waiting for new stream", ServerLogPrefix)
				stream, err := sess.AcceptStream(ctx)
				if err != nil {
					if c != nil {
						// if client close the connection, then we should close the session
						app, ok := s.connector.App(c.ConnID)
						if ok {
							// connector
							s.connector.Remove(c.ConnID)
							// store
							// when remove store by appID? let me think...
							logger.Errorf("%s❤️3/ [%s::%s](%s) on stream %v", ServerLogPrefix, app.ID(), app.Name(), c.ConnID, err)
							logger.Printf("%s💔 [%s::%s](%s) is disconnected", ServerLogPrefix, app.ID(), app.Name(), c.ConnID)
						} else {
							logger.Errorf("%s❤️3/ [unknown](%s) on stream %v", ServerLogPrefix, c.ConnID, err)
						}
					}
					break
				}
				defer stream.Close()

				logger.Infof("%s❤️4/ [stream:%d] created, %s", ServerLogPrefix, stream.StreamID(), sess.RemoteAddr().String())
				// process frames on stream
				c = newContext(stream)
				defer c.Clean()
				s.handleSession(c)
				logger.Infof("%s❤️5/ [stream:%d] handleSession DONE", ServerLogPrefix, stream.StreamID())
			}
		}(sctx, session)
	}
}

// Close will shutdown the server.
func (s *Server) Close() error {
	// if s.stream != nil {
	// 	if err := s.stream.Close(); err != nil {
	// 		logger.Errorf("%sClose(): %v", ServerLogPrefix, err)
	// 		return err
	// 	}
	// }
	// router
	if s.router != nil {
		s.router.Clean()
	}
	// connector
	if s.connector != nil {
		s.connector.Clean()
	}
	// store
	if s.opts.Store != nil {
		s.opts.Store.Clean()
	}
	return nil
}

// handle streams on a session
func (s *Server) handleSession(c *Context) {
	fs := NewFrameStream(c.Stream)
	// check update for stream
	for {
		logger.Debugf("%shandleSession 💚 waiting read next...", ServerLogPrefix)
		f, err := fs.ReadFrame()
		if err != nil {
			logger.Errorf("%s [ERR] %T %v", ServerLogPrefix, err, err)
			if errors.Is(err, net.ErrClosed) {
				// if client close the connection, net.ErrClosed will be raise
				// by quic-go IdleTimeoutError after connection's KeepAlive config.
				logger.Warnf("%s [ERR] net.ErrClosed on [handleSession] %v", ServerLogPrefix, net.ErrClosed)
				c.CloseWithError(0xC1, "net.ErrClosed")
				break
			}
			// any error occurred, we should close the session
			// after this, session.AcceptStream() will raise the error
			// which specific in session.CloseWithError()
			c.CloseWithError(0xC0, err.Error())
			logger.Warnf("%ssession.Close()", ServerLogPrefix)
			break
		}

		frameType := f.Type()
		logger.Debugf("%stype=%s, frame=%# x", ServerLogPrefix, frameType, f.Encode())
		// add frame to context
		c := c.WithFrame(f)

		// before frame handlers
		for _, handler := range s.beforeHandlers {
			if err := handler(c); err != nil {
				logger.Errorf("%sbeforeFrameHandler err: %s", ServerLogPrefix, err)
				c.CloseWithError(0xCC, err.Error())
				return
			}
		}
		// main handler
		if err := s.mainFrameHandler(c); err != nil {
			logger.Errorf("%smainFrameHandler err: %s", ServerLogPrefix, err)
			c.CloseWithError(0xCC, err.Error())
			return
		}
		// after frame handler
		for _, handler := range s.afterHandlers {
			if err := handler(c); err != nil {
				logger.Errorf("%safterFrameHandler err: %s", ServerLogPrefix, err)
				c.CloseWithError(0xCC, err.Error())
				return
			}
		}
	}
}

func (s *Server) mainFrameHandler(c *Context) error {
	var err error
	frameType := c.Frame.Type()

	switch frameType {
	case frame.TagOfHandshakeFrame:
		if err := s.handleHandshakeFrame(c); err != nil {
			logger.Errorf("%shandleHandshakeFrame err: %s", ServerLogPrefix, err)
			c.CloseWithError(0xCC, err.Error())
			// break
		}
	// case frame.TagOfPingFrame:
	// 	s.handlePingFrame(mainStream, session, f.(*frame.PingFrame))
	case frame.TagOfDataFrame:
		if err := s.handleDataFrame(c); err != nil {
			c.CloseWithError(0xCC, "处理DataFrame出错")
		} else {
			s.dispatchToDownstreams(c.Frame.(*frame.DataFrame))
		}
	default:
		logger.Errorf("%serr=%v, frame=%v", ServerLogPrefix, err, c.Frame.Encode())
	}
	return nil
}

// handle HandShakeFrame
func (s *Server) handleHandshakeFrame(c *Context) error {
	f := c.Frame.(*frame.HandshakeFrame)
	if len(f.InstanceID) == 0 {
		return errors.New("handleHandshakeFrame f.InstanceID is empty")
	} else if len(c.ConnID) > 0 {
		return errors.New("handleHandshakeFrame c.ConnID is not empty")
	}
	c.SetConnID(f.InstanceID)

	logger.Debugf("%sGOT ❤️ HandshakeFrame : %# x", ServerLogPrefix, f)
	// credential
	logger.Infof("%sClientType=%# x is %s, CredentialType=%s", ServerLogPrefix, f.ClientType, ClientType(f.ClientType), auth.AuthType(f.AuthType()))
	// authenticate
	if !s.authenticate(f) {
		err := fmt.Errorf("handshake authentication fails, client credential type is %s", auth.AuthType(f.AuthType()))
		return err
	}

	// route
	appID := f.AppID()
	if err := s.validateRouter(); err != nil {
		return err
	}
	connID := c.ConnID
	route := s.router.Route(appID)
	if reflect.ValueOf(route).IsNil() {
		err := errors.New("handleHandshakeFrame route is nil")
		return err
	}
	// store
	s.opts.Store.Set(appID, route)

	// client type
	clientType := ClientType(f.ClientType)
	name := f.Name
	stream := c.Stream
	switch clientType {
	case ClientTypeSource:
		s.connector.Add(connID, stream)
		s.connector.LinkApp(connID, appID, name)
	case ClientTypeStreamFunction:
		// when sfn connect, it will provide its name to the server. server will check if this client
		// has permission connected to.
		if !route.Exists(name) {
			// unexpected client connected, close the connection
			s.connector.Remove(connID)
			// SFN: stream function
			err := fmt.Errorf("handshake router validation faild, illegal SFN[%s]", f.Name)
			c.CloseWithError(0xCC, err.Error())
			// break
			return err
		}

		s.connector.Add(connID, stream)
		// link connection to stream function
		s.connector.LinkApp(connID, appID, name)
	case ClientTypeUpstreamZipper:
		s.connector.Add(connID, stream)
		s.connector.LinkApp(connID, appID, name)
	default:
		// unknown client type
		s.connector.Remove(connID)
		logger.Errorf("%sClientType=%# x, ilegal!", ServerLogPrefix, f.ClientType)
		c.CloseWithError(0xCD, "Unknown ClientType, illegal!")
		return errors.New("core.server: Unknown ClientType, illegal")
	}
	logger.Printf("%s❤️  <%s> [%s::%s](%s) is connected!", ServerLogPrefix, clientType, appID, name, connID)
	return nil
}

// will reuse quic-go's keep-alive feature
// func (s *Server) handlePingFrame(stream quic.Stream, session quic.Session, f *frame.PingFrame) error {
// 	logger.Infof("%s------> GOT ❤️ PingFrame : %# x", ServerLogPrefix, f)
// 	return nil
// }

func (s *Server) handleDataFrame(c *Context) error {
	// counter +1
	atomic.AddInt64(&s.counterOfDataFrame, 1)
	// currentIssuer := f.GetIssuer()
	fromID := c.ConnID
	from, ok := s.connector.AppName(fromID)
	if !ok {
		logger.Warnf("%shandleDataFrame have connection[%s], but not have function", ServerLogPrefix, fromID)
		return nil
	}

	// route
	appID, _ := s.connector.AppID(fromID)
	cacheRoute, ok := s.opts.Store.Get(appID)
	if !ok {
		err := fmt.Errorf("get route failure, appID=%s, connID=%s", appID, fromID)
		logger.Errorf("%shandleDataFrame %s", ServerLogPrefix, err.Error())
		return err
	}
	route := cacheRoute.(Route)
	if route == nil {
		logger.Warnf("%shandleDataFrame route is nil", ServerLogPrefix)
		return fmt.Errorf("handleDataFrame route is nil")
	}
	// get stream function name from route
	to, ok := route.Next(from)
	if !ok {
		logger.Warnf("%shandleDataFrame have not next function, from=[%s](%s)", ServerLogPrefix, from, fromID)
		return nil
	}
	f := c.Frame.(*frame.DataFrame)
	// write data frame to stream
	toIDs := s.connector.GetConnIDs(appID, to, f.GetMetaFrame())
	for _, toID := range toIDs {
		logger.Infof("%swrite data: [%s](%s) --> [%s](%s)", ServerLogPrefix, from, fromID, to, toID)
		if err := s.connector.Write(f, toID); err != nil {
			logger.Errorf("%swrite data: [%s](%s) --> [%s](%s), err=%v", ServerLogPrefix, from, fromID, to, toID, err)
			return err
		}
	}
	return nil
}

// StatsFunctions returns the sfn stats of server.
// func (s *Server) StatsFunctions() map[string][]*quic.Stream {
func (s *Server) StatsFunctions() map[string]io.ReadWriteCloser {
	return s.connector.GetSnapshot()
}

// StatsCounter returns how many DataFrames pass through server.
func (s *Server) StatsCounter() int64 {
	return s.counterOfDataFrame
}

// Downstreams return all the downstream servers.
func (s *Server) Downstreams() map[string]*Client {
	return s.downstreams
}

// AddWorkflow register sfn to this server.
// func (s *Server) AddWorkflow(wfs ...Workflow) error {
// 	for _, wf := range wfs {
// 		s.router.Add(wf.Seq, wf.Name)
// 	}
// 	return nil
// }

func (s *Server) ConfigRouter(router Router) error {
	s.mu.Lock()
	s.router = router
	logger.Debugf("%sconfig router is %#v", ServerLogPrefix, router)
	s.mu.Unlock()
	return nil
}

func (s *Server) Router() Router {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.router
}

func (s *Server) init() {
	// // tracing
	// _, _, err := tracing.NewTracerProvider(s.name)
	// if err != nil {
	// 	logger.Errorf("tracing: %v", err)
	// }
}

// AddDownstreamServer add a downstream server to this server. all the DataFrames will be
// dispatch to all the downstreams.
func (s *Server) AddDownstreamServer(addr string, c *Client) {
	s.mu.Lock()
	s.downstreams[addr] = c
	s.mu.Unlock()
}

// dispatch every DataFrames to all downstreams
func (s *Server) dispatchToDownstreams(df *frame.DataFrame) {
	for addr, ds := range s.downstreams {
		logger.Debugf("%sdispatching to [%s]: %# x", ServerLogPrefix, addr, df.Tag())
		ds.WriteFrame(df)
	}
}

func (s *Server) initOptions() {
	// defaults
	// store
	if s.opts.Store == nil {
		s.opts.Store = store.NewMemoryStore()
	}
	// auth
	if s.opts.Auths == nil {
		s.opts.Auths = append(s.opts.Auths, auth.NewAuthNone())
	}
}

func (s *Server) validateRouter() error {
	if s.router == nil {
		return errors.New("server's router is nil")
	}
	return nil
}

func (s *Server) Options() ServerOptions {
	return s.opts
}

func (s *Server) Connector() Connector {
	return s.connector
}

func (s *Server) Store() store.Store {
	return s.opts.Store
}

func (s *Server) SetBeforeHandlers(handlers ...FrameHandler) {
	s.beforeHandlers = append(s.beforeHandlers, handlers...)
}

func (s *Server) SetAfterHandlers(handlers ...FrameHandler) {
	s.afterHandlers = append(s.afterHandlers, handlers...)
}

func (s *Server) authNames() []string {
	result := []string{}
	for _, auth := range s.opts.Auths {
		result = append(result, auth.Type().String())
	}
	return result
}

func (s *Server) authenticate(f *frame.HandshakeFrame) bool {
	if len(s.opts.Auths) > 0 {
		for _, auth := range s.opts.Auths {
			isAuthenticated := auth.Authenticate(f)
			if isAuthenticated {
				logger.Debugf("%sauthenticate: [%s]=%v", ServerLogPrefix, auth.Type(), isAuthenticated)
				return isAuthenticated
			}
		}
		return false
	}
	return true
}
