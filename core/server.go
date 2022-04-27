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
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/store"
	"github.com/yomorun/yomo/core/yerr"

	// authentication implements, Currently, only token authentication is implemented
	_ "github.com/yomorun/yomo/pkg/auth"
	"github.com/yomorun/yomo/pkg/logger"
	pkgtls "github.com/yomorun/yomo/pkg/tls"
)

const (
	DefaultListenAddr = "0.0.0.0:9000"
)

type ServerOption func(*ServerOptions)

// type FrameHandler func(store store.Store, stream quic.Stream, conn quic.Connection, f frame.Frame) error
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
		logger.Errorf("%slistener.Listen: err=%v", ServerLogPrefix, err)
		return err
	}
	defer listener.Close()
	logger.Printf("%s✅ [%s] Listening on: %s, MODE: %s, QUIC: %v, AUTH: %s", ServerLogPrefix, s.name, listener.Addr(), mode(), listener.Versions(), s.authNames())

	s.state = ConnStateConnected
	for {
		// create a new connection when new yomo-client connected
		sctx, cancel := context.WithCancel(ctx)
		defer cancel()

		conn, err := listener.Accept(sctx)
		if err != nil {
			logger.Errorf("%screate connection error: %v", ServerLogPrefix, err)
			return err
		}

		connID := GetConnID(conn)
		logger.Infof("%s❤️1/ new connection: %s", ServerLogPrefix, connID)

		go func(ctx context.Context, conn quic.Connection) {
			for {
				logger.Infof("%s❤️2/ waiting for new stream", ServerLogPrefix)
				stream, err := conn.AcceptStream(ctx)
				if err != nil {
					// if client close the connection, then we should close the connection
					// @CC: when Source close the connection, it won't affect connectors
					s.connector.Remove(connID)
					// appName, ok := s.connector.AppName(connID)
					// if ok {
					// 	// connector
					// 	s.connector.Remove(connID)
					// store
					// remove app route from store? let me think...
					// logger.Printf("%s💔 [%s](%s) close the connection", ServerLogPrefix, appName, connID)
					logger.Printf("%s💔 (%s) close the connection", ServerLogPrefix, connID)
					// } else {
					// 	logger.Errorf("%s❤️3/ [unknown](%s) on stream %v", ServerLogPrefix, connID, err)
					// }
					break
				}
				defer stream.Close()

				logger.Infof("%s❤️4/ [stream:%d] created, connID=%s", ServerLogPrefix, stream.StreamID(), connID)
				// process frames on stream
				// c := newContext(connID, stream)
				c := newContext(conn, stream)
				defer c.Clean()
				s.handleConnection(c)
				logger.Infof("%s❤️5/ [stream:%d] handleConnection DONE", ServerLogPrefix, stream.StreamID())
			}
		}(sctx, conn)
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

// handle streams on a connection
func (s *Server) handleConnection(c *Context) {
	fs := NewFrameStream(c.Stream)
	// check update for stream
	for {
		logger.Debugf("%shandleConnection 💚 waiting read next...", ServerLogPrefix)
		f, err := fs.ReadFrame()
		if err != nil {
			// if client close connection, will get ApplicationError with code = 0x00
			if e, ok := err.(*quic.ApplicationError); ok {
				if yerr.Is(e.ErrorCode, yerr.ErrorCodeClientAbort) {
					// client abort
					logger.Infof("%sclient close the connection", ServerLogPrefix)
					break
				} else {
					ye := yerr.New(yerr.Parse(e.ErrorCode), err)
					logger.Errorf("%s[ERR] %s", ServerLogPrefix, ye)
				}
			} else if err == io.EOF {
				logger.Infof("%sthe connection is EOF", ServerLogPrefix)
				break
			}
			if errors.Is(err, net.ErrClosed) {
				// if client close the connection, net.ErrClosed will be raise
				// by quic-go IdleTimeoutError after connection's KeepAlive config.
				logger.Warnf("%s[ERR] net.ErrClosed on [handleConnection] %v", ServerLogPrefix, net.ErrClosed)
				c.CloseWithError(yerr.ErrorCodeClosed, "net.ErrClosed")
				break
			}
			// any error occurred, we should close the stream
			// after this, conn.AcceptStream() will raise the error
			c.CloseWithError(yerr.ErrorCodeUnknown, err.Error())
			logger.Warnf("%sconnection.Close()", ServerLogPrefix)
			break
		}

		frameType := f.Type()
		data := f.Encode()
		logger.Debugf("%stype=%s, frame[%d]=%# x", ServerLogPrefix, frameType, len(data), frame.Shortly(data))
		// add frame to context
		c := c.WithFrame(f)

		// before frame handlers
		for _, handler := range s.beforeHandlers {
			if err := handler(c); err != nil {
				logger.Errorf("%safterFrameHandler err: %s", ServerLogPrefix, err)
				c.CloseWithError(yerr.ErrorCodeBeforeHandler, err.Error())
				return
			}
		}
		// main handler
		if err := s.mainFrameHandler(c); err != nil {
			logger.Errorf("%smainFrameHandler err: %s", ServerLogPrefix, err)
			c.CloseWithError(yerr.ErrorCodeMainHandler, err.Error())
			return
		}
		// after frame handler
		for _, handler := range s.afterHandlers {
			if err := handler(c); err != nil {
				logger.Errorf("%safterFrameHandler err: %s", ServerLogPrefix, err)
				c.CloseWithError(yerr.ErrorCodeAfterHandler, err.Error())
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
			// c.CloseWithError(0xCC, err.Error())
			// return err
			return yerr.New(yerr.ErrorCodeHandshake, err)
			// break
		}
	// case frame.TagOfPingFrame:
	// 	s.handlePingFrame(mainStream, connection, f.(*frame.PingFrame))
	case frame.TagOfGoawayFrame:
		if err := s.handleGoawayFrame(c); err != nil {
			// return err
			return yerr.New(yerr.ErrorCodeGoaway, err)
		}
	case frame.TagOfDataFrame:
		if err := s.handleDataFrame(c); err != nil {
			c.CloseWithError(yerr.ErrorCodeData, fmt.Sprintf("handleDataFrame err: %v", err))
		} else {
			// observe datatags backflow
			s.handleBackflowFrame(c)
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

	logger.Debugf("%sGOT ❤️ HandshakeFrame : %# x", ServerLogPrefix, f)
	// basic info
	connID := c.ConnID()
	name := f.Name
	clientType := ClientType(f.ClientType)
	stream := c.Stream
	// credential
	logger.Debugf("%sClientType=%# x is %s, Credential=%s", ServerLogPrefix, f.ClientType, ClientType(f.ClientType), authName(f.AuthName()))
	// authenticate
	if !s.authenticate(f) {
		err := fmt.Errorf("handshake authentication fails, client credential name is %s", authName(f.AuthName()))
		// return err
		logger.Debugf("%s🔑 <%s> [%s](%s) is connected!", ServerLogPrefix, clientType, name, connID)
		rejectedFrame := frame.NewRejectedFrame(err.Error())
		if _, err = stream.Write(rejectedFrame.Encode()); err != nil {
			logger.Debugf("%s🔑 write to <%s> [%s](%s) RejectedFrame error:%v", ServerLogPrefix, clientType, name, connID, err)
			return err
		}
		return nil
	}

	// route
	if err := s.validateRouter(); err != nil {
		return err
	}
	route := s.router.Route()
	if reflect.ValueOf(route).IsNil() {
		err := errors.New("handleHandshakeFrame route is nil")
		return err
	}
	// store
	s.opts.Store.Set(name, route)

	// client type
	switch clientType {
	case ClientTypeSource:
		s.connector.Add(connID, stream)
		s.connector.LinkApp(connID, name, nil)
		s.connector.LinkSource(connID, name, f.ObserveDataTags)
	case ClientTypeStreamFunction:
		// when sfn connect, it will provide its name to the server. server will check if this client
		// has permission connected to.
		if !route.Exists(name) {
			// unexpected client connected, close the connection
			s.connector.Remove(connID)
			// SFN: stream function
			err := fmt.Errorf("handshake router validation faild, illegal SFN[%s]", f.Name)
			c.CloseWithError(yerr.ErrorCodeRejected, err.Error())
			// break
			return err
		}
		// check app exists in connection list
		if s.connector.ExistsApp(name) {
			logger.Debugf("%swrite to SFN[%s] GoawayFrame", ServerLogPrefix, f.Name)
			err := fmt.Errorf("SFN[%s] connection already exists", f.Name)
			goawayFrame := frame.NewGoawayFrame(err.Error())
			if _, err = stream.Write(goawayFrame.Encode()); err != nil {
				logger.Errorf("%s⛔️ write to SFN[%s] GoawayFrame error:%v", ServerLogPrefix, f.Name, err)
				return err
			}
			// c.CloseWithError(goawayFrame.Code(), err.Error())
		}
		if s.connector.ExistsApp(name) {
			logger.Debugf("%swrite to SFN[%s] GoawayFrame", ServerLogPrefix, f.Name)
			err := fmt.Errorf("SFN[%s] connection already exists", f.Name)
			goawayFrame := frame.NewGoawayFrame(err.Error())
			if _, err = stream.Write(goawayFrame.Encode()); err != nil {
				logger.Errorf("%s⛔️ write to SFN[%s] GoawayFrame error:%v", ServerLogPrefix, f.Name, err)
				return err
			}
		}
		s.connector.Add(connID, stream)
		// link connection to stream function
		s.connector.LinkApp(connID, name, f.ObserveDataTags)
	case ClientTypeUpstreamZipper:
		s.connector.Add(connID, stream)
		s.connector.LinkApp(connID, name, nil)
	default:
		// unknown client type
		s.connector.Remove(connID)
		logger.Errorf("%sClientType=%# x, ilegal!", ServerLogPrefix, f.ClientType)
		c.CloseWithError(yerr.ErrorCodeUnknownClient, "Unknown ClientType, illegal!")
		return errors.New("core.server: Unknown ClientType, illegal")
	}
	logger.Printf("%s❤️  <%s> [%s](%s) is connected!", ServerLogPrefix, clientType, name, connID)
	return nil
}

// handle handleGoawayFrame
func (s *Server) handleGoawayFrame(c *Context) error {
	f := c.Frame.(*frame.GoawayFrame)

	logger.Debugf("%s⛔️ GOT GoawayFrame code=%d, message==%s", ServerLogPrefix, yerr.ErrorCodeGoaway, f.Message())
	// c.CloseWithError(f.Code(), f.Message())
	_, err := c.Stream.Write(f.Encode())
	return err
}

// will reuse quic-go's keep-alive feature
// func (s *Server) handlePingFrame(stream quic.Stream, conn quic.Connection, f *frame.PingFrame) error {
// 	logger.Infof("%s------> GOT ❤️ PingFrame : %# x", ServerLogPrefix, f)
// 	return nil
// }

func (s *Server) handleDataFrame(c *Context) error {
	// counter +1
	atomic.AddInt64(&s.counterOfDataFrame, 1)
	// currentIssuer := f.GetIssuer()
	fromID := c.ConnID()
	from, ok := s.connector.AppName(fromID)
	if !ok {
		logger.Warnf("%shandleDataFrame have connection[%s], but not have function", ServerLogPrefix, fromID)
		return nil
	}

	f := c.Frame.(*frame.DataFrame)

	// route
	name, _ := s.connector.AppName(fromID)
	cacheRoute, ok := s.opts.Store.Get(name)
	if !ok {
		err := fmt.Errorf("get route failure, appName=%s, connID=%s", name, fromID)
		logger.Errorf("%shandleDataFrame %s", ServerLogPrefix, err.Error())
		return err
	}
	route := cacheRoute.(Route)
	if route == nil {
		logger.Warnf("%shandleDataFrame route is nil", ServerLogPrefix)
		return fmt.Errorf("handleDataFrame route is nil")
	}
	// get stream function names from route
	routes := route.GetForwardRoutes(from)
	for _, to := range routes {
		toIDs := s.connector.GetConnIDs(to, f.GetDataTag())
		for _, toID := range toIDs {
			logger.Debugf("%shandleDataFrame tag=%#x tid=%s, counter=%d, from=[%s](%s), to=[%s](%s)", ServerLogPrefix, f.Tag(), f.TransactionID(), s.counterOfDataFrame, from, fromID, to, toID)

			// write data frame to stream
			logger.Infof("%swrite data: [%s](%s) --> [%s](%s)", ServerLogPrefix, from, fromID, to, toID)
			if err := s.connector.Write(f, toID); err != nil {
				logger.Errorf("%swrite data: [%s](%s) --> [%s](%s), err=%v", ServerLogPrefix, from, fromID, to, toID, err)
				continue
			}
		}
	}
	return nil
}

func (s *Server) handleBackflowFrame(c *Context) error {
	f := c.Frame.(*frame.DataFrame)
	tag := f.GetDataTag()
	carriage := f.GetCarriage()
	// write to source with BackflowFrame
	bf := frame.NewBackflowFrame(tag, carriage)
	sourceIDs := s.connector.GetSourceConnIDs(tag)
	for _, sourceID := range sourceIDs {
		logger.Debugf("%shandleBackflowFrame tag:%#v --> source:%s, result=%# x", ServerLogPrefix, tag, sourceID, frame.Shortly(carriage))
		// get source's quic.Stream
		source := s.connector.Get(sourceID)
		if source != nil {
			_, err := source.Write(bf.Encode())
			if err != nil {
				logger.Errorf("%shandleBackflowFrame tag:%#v --> source:%s, error=%v", ServerLogPrefix, tag, sourceID, err)
				return err
			}
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

// GetConnID get quic connection id
func GetConnID(conn quic.Connection) string {
	return conn.RemoteAddr().String()
}

func (s *Server) initOptions() {
	// defaults
	// store
	if s.opts.Store == nil {
		s.opts.Store = store.NewMemoryStore()
	}
	// auth
	// if s.opts.Auths == nil {
	// 	s.opts.Auths = append(s.opts.Auths, auth.NewNoneAuth())
	// }
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
	if len(s.opts.Auths) == 0 {
		return []string{"none"}
	}
	result := []string{}
	for _, auth := range s.opts.Auths {
		result = append(result, auth.Name())
	}
	return result
}

func (s *Server) authenticate(f *frame.HandshakeFrame) bool {
	if len(s.opts.Auths) > 0 {
		for _, auth := range s.opts.Auths {
			isAuthenticated := auth.Authenticate(f.AuthPayload())
			if isAuthenticated {
				logger.Debugf("%sauthenticated==%v", ServerLogPrefix, isAuthenticated)
				return isAuthenticated
			}
		}
		return false
	}
	return true
}

func mode() string {
	if pkgtls.IsDev() {
		return "DEVELOPMENT"
	}
	return "PRODUCTION"
}

func authName(name string) string {
	if name == "" {
		return "empty"
	}

	return name
}
