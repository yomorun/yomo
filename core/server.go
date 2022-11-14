package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"

	"github.com/lucas-clemente/quic-go"
	"github.com/yomorun/yomo/core/auth"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/yerr"

	// authentication implements, Currently, only token authentication is implemented
	_ "github.com/yomorun/yomo/pkg/auth"
	"github.com/yomorun/yomo/pkg/logger"
)

const (
	// DefaultListenAddr is the default address to listen.
	DefaultListenAddr = "0.0.0.0:9000"
)

// ServerOption is the option for server.
type ServerOption func(*ServerOptions)

// FrameHandler is the handler for frame.
type FrameHandler func(c *Context) error

// ConnectionHandler is the handler for quic connection
type ConnectionHandler func(conn quic.Connection)

// Server is the underlining server of Zipper
type Server struct {
	name                    string
	connector               Connector
	router                  Router
	metadataBuilder         MetadataBuilder
	alpnHandler             func(proto string) error
	counterOfDataFrame      int64
	downstreams             map[string]*Client
	mu                      sync.Mutex
	opts                    ServerOptions
	beforeHandlers          []FrameHandler
	afterHandlers           []FrameHandler
	connectionCloseHandlers []ConnectionHandler
	listener                Listener
	wg                      *sync.WaitGroup
}

// NewServer create a Server instance.
func NewServer(name string, opts ...ServerOption) *Server {
	s := &Server{
		name:        name,
		connector:   newConnector(),
		downstreams: make(map[string]*Client),
		wg:          new(sync.WaitGroup),
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
		return err
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}
	return s.Serve(ctx, conn)
}

// Serve the server with a net.PacketConn.
func (s *Server) Serve(ctx context.Context, conn net.PacketConn) error {
	if err := s.validateMetadataBuilder(); err != nil {
		return err
	}

	if err := s.validateRouter(); err != nil {
		return err
	}

	// listen the address
	listener, err := newListener(conn, s.opts.TLSConfig, s.opts.QuicConfig)
	if err != nil {
		logger.Errorf("%slistener.Listen: err=%v", ServerLogPrefix, err)
		return err
	}
	s.listener = listener
	// defer listener.Close()
	logger.Printf("%s✅ [%s][%d] Listening on: %s, QUIC: %v, AUTH: %s", ServerLogPrefix, s.name, os.Getpid(), listener.Addr(), listener.Versions(), s.authNames())

	for {
		// create a new connection when new yomo-client connected
		sctx, cancel := context.WithCancel(ctx)
		defer cancel()

		conn, err := s.listener.Accept(sctx)
		if err != nil {
			logger.Errorf("%slistener accept connections error: %v", ServerLogPrefix, err)
			return err
		}
		err = s.alpnHandler(conn.ConnectionState().TLS.NegotiatedProtocol)
		if err != nil {
			conn.CloseWithError(quic.ApplicationErrorCode(yerr.ErrorCodeRejected), err.Error())
			continue
		}

		// connection close handlers on server shutdown
		// defer s.doConnectionCloseHandlers(conn)
		s.wg.Add(1)
		connID := GetConnID(conn)
		logger.Infof("%s❤️1/ new connection: %s", ServerLogPrefix, connID)

		go func(ctx context.Context, qconn quic.Connection) {
			// connection close handlers on client connect timeout
			defer s.doConnectionCloseHandlers(qconn)
			for {
				logger.Infof("%s❤️2/ waiting for new stream", ServerLogPrefix)
				stream, err := qconn.AcceptStream(ctx)
				if err != nil {
					// if client close the connection, then we should close the connection
					// @CC: when Source close the connection, it won't affect connectors
					name := "-"
					clientID := "-"
					if conn := s.connector.Get(connID); conn != nil {
						// connector
						s.connector.Remove(connID)
						route := s.router.Route(conn.Metadata())
						if route != nil {
							route.Remove(connID)
						}
						name = conn.Name()
						clientID = conn.ClientID()
						conn.Close()
					}
					logger.Printf("%s💔 [%s][%s](%s) close the connection: %v", ServerLogPrefix, name, clientID, connID, err)
					break
				}
				defer stream.Close()

				logger.Infof("%s❤️3/ [stream:%d] created, connID=%s", ServerLogPrefix, stream.StreamID(), connID)
				// process frames on stream
				c := newContext(conn, stream)
				defer c.Clean()
				s.handleConnection(c)
				logger.Infof("%s❤️4/ [stream:%d] handleConnection DONE", ServerLogPrefix, stream.StreamID())
			}
		}(sctx, conn)
	}
}

// Close will shutdown the server.
func (s *Server) Close() error {
	// listener
	if s.listener != nil {
		s.listener.Close()
	}
	// router
	if s.router != nil {
		s.router.Clean()
	}
	// connector
	if s.connector != nil {
		s.connector.Clean()
	}
	s.wg.Wait()
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
				logger.Errorf("%sbeforeFrameHandler err: %s", ServerLogPrefix, err)
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
			// close connections early to avoid resource consumption
			if c.Stream != nil {
				goawayFrame := frame.NewGoawayFrame(err.Error())
				if _, err := c.Stream.Write(goawayFrame.Encode()); err != nil {
					logger.Errorf("%s⛔️ write to client[%s] GoawayFrame error:%v", ServerLogPrefix, c.ConnID, err)
					return err
				}
			}
		}
	// case frame.TagOfPingFrame:
	// 	s.handlePingFrame(mainStream, connection, f.(*frame.PingFrame))
	case frame.TagOfDataFrame:
		if err := s.handleDataFrame(c); err != nil {
			c.CloseWithError(yerr.ErrorCodeData, fmt.Sprintf("handleDataFrame err: %v", err))
		} else {
			conn := s.connector.Get(c.connID)
			if conn != nil && conn.ClientType() == ClientTypeSource {
				f := c.Frame.(*frame.DataFrame)
				if f.IsBroadcast() {
					f.GetMetaFrame().SetMetadata(conn.Metadata().Encode())
					s.dispatchToDownstreams(f)
				}
			}
			// observe datatags backflow
			s.handleBackflowFrame(c)
		}
	default:
		logger.Errorf("%serr=%v, frame=%v", ServerLogPrefix, err, frame.Shortly(c.Frame.Encode()))
	}
	return nil
}

// handle HandShakeFrame
func (s *Server) handleHandshakeFrame(c *Context) error {
	f := c.Frame.(*frame.HandshakeFrame)

	logger.Debugf("%sGOT ❤️ HandshakeFrame : %# x", ServerLogPrefix, f)
	// basic info
	connID := c.ConnID()
	clientID := f.ClientID
	clientType := ClientType(f.ClientType)
	stream := c.Stream
	// credential
	logger.Debugf("%sClientType=%# x is %s, ClientID=%s, Credential=%s", ServerLogPrefix, f.ClientType, ClientType(f.ClientType), clientID, authName(f.AuthName()))
	// authenticate
	authed := auth.Authenticate(s.opts.Auths, f)
	logger.Debugf("%sauthenticated==%v", ServerLogPrefix, authed)
	if !authed {
		err := fmt.Errorf("handshake authentication fails, client credential name is %s", authName(f.AuthName()))
		// return err
		logger.Debugf("%s🔑 <%s> [%s](%s) is connected!", ServerLogPrefix, clientType, f.Name, connID)
		rejectedFrame := frame.NewRejectedFrame(err.Error())
		if _, err = stream.Write(rejectedFrame.Encode()); err != nil {
			logger.Debugf("%s🔑 write to <%s> [%s](%s) RejectedFrame error:%v", ServerLogPrefix, clientType, f.Name, connID, err)
			return err
		}
		return nil
	}

	// client type
	var conn Connection
	switch clientType {
	case ClientTypeSource, ClientTypeStreamFunction:
		// metadata
		metadata, err := s.metadataBuilder.Build(f)
		if err != nil {
			return err
		}
		conn = newConnection(f.Name, f.ClientID, clientType, metadata, stream, f.ObserveDataTags)

		if clientType == ClientTypeStreamFunction {
			// route
			route := s.router.Route(metadata)
			if route == nil {
				return errors.New("handleHandshakeFrame route is nil")
			}
			if err := route.Add(connID, f.Name, f.ObserveDataTags); err != nil {
				// duplicate name
				if e, ok := err.(yerr.DuplicateNameError); ok {
					existsConnID := e.ConnID()
					if conn := s.connector.Get(existsConnID); conn != nil {
						logger.Debugf("%s%s, write to SFN[%s](%s) GoawayFrame", ServerLogPrefix, e.Error(), f.Name, existsConnID)
						goawayFrame := frame.NewGoawayFrame(e.Error())
						if err := conn.Write(goawayFrame); err != nil {
							logger.Errorf("%s⛔️ write to SFN[%s] GoawayFrame error:%v", ServerLogPrefix, f.Name, err)
							return err
						}
					}
				} else {
					return err
				}
			}
		}
	case ClientTypeUpstreamZipper:
		conn = newConnection(f.Name, f.ClientID, clientType, nil, stream, f.ObserveDataTags)
	default:
		// unknown client type
		s.connector.Remove(connID)
		err := fmt.Errorf("illegal ClientType: %#x", f.ClientType)
		c.CloseWithError(yerr.ErrorCodeUnknownClient, err.Error())
		return err
	}

	s.connector.Add(connID, conn)
	logger.Printf("%s❤️  <%s> [%s][%s](%s) is connected!", ServerLogPrefix, clientType, f.Name, clientID, connID)
	return nil
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
	from := s.connector.Get(fromID)
	if from == nil {
		logger.Warnf("%shandleDataFrame connector cannot find %s", ServerLogPrefix, fromID)
		return fmt.Errorf("handleDataFrame connector cannot find %s", fromID)
	}

	f := c.Frame.(*frame.DataFrame)

	var metadata Metadata
	if from.ClientType() == ClientTypeUpstreamZipper {
		m, err := s.metadataBuilder.Decode(f.GetMetaFrame().Metadata())
		if err != nil {
			return err
		}
		metadata = m
	} else {
		metadata = from.Metadata()
	}

	// route
	route := s.router.Route(metadata)
	if route == nil {
		logger.Warnf("%shandleDataFrame route is nil", ServerLogPrefix)
		return fmt.Errorf("handleDataFrame route is nil")
	}

	// get stream function connection ids from route
	connIDs := route.GetForwardRoutes(f.GetDataTag())
	for _, toID := range connIDs {
		conn := s.connector.Get(toID)
		if conn == nil {
			logger.Errorf("%sconn is nil: (%s)", ServerLogPrefix, toID)
			continue
		}

		to := conn.Name()
		logger.Debugf("%shandleDataFrame tag=%#x tid=%s, counter=%d, from=[%s](%s), to=[%s](%s)", ServerLogPrefix, f.Tag(), f.TransactionID(), s.counterOfDataFrame, from.Name(), fromID, to, toID)

		// write data frame to stream
		if err := conn.Write(f); err != nil {
			logger.Warnf("%shandleDataFrame conn.Write tag=%#x tid=%s, from=[%s](%s), to=[%s](%s), %v", ServerLogPrefix, f.Tag(), f.TransactionID(), from.Name(), fromID, to, toID, err)
		}
	}

	return nil
}

func (s *Server) handleBackflowFrame(c *Context) error {
	f := c.Frame.(*frame.DataFrame)
	tag := f.GetDataTag()
	carriage := f.GetCarriage()
	sourceID := f.SourceID()
	// write to source with BackflowFrame
	bf := frame.NewBackflowFrame(tag, carriage)
	sourceConns := s.connector.GetSourceConns(sourceID, tag)
	// conn := s.connector.Get(c.connID)
	// logger.Printf("%s♻️  handleBackflowFrame tag:%#v --> source:%s, result=%s", ServerLogPrefix, tag, sourceID, carriage)
	for _, source := range sourceConns {
		if source != nil {
			logger.Debugf("%s♻️  handleBackflowFrame tag:%#v --> source:%s, result=%# x", ServerLogPrefix, tag, sourceID, frame.Shortly(carriage))
			if err := source.Write(bf); err != nil {
				logger.Errorf("%s♻️  handleBackflowFrame tag:%#v --> source:%s, error=%v", ServerLogPrefix, tag, sourceID, err)
				return err
			}
		}
	}
	return nil
}

// StatsFunctions returns the sfn stats of server.
func (s *Server) StatsFunctions() map[string]string {
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

// ConfigRouter is used to set router by zipper
func (s *Server) ConfigRouter(router Router) {
	s.mu.Lock()
	s.router = router
	logger.Debugf("%sconfig router is %#v", ServerLogPrefix, router)
	s.mu.Unlock()
}

// ConfigMetadataBuilder is used to set metadataBuilder by zipper
func (s *Server) ConfigMetadataBuilder(builder MetadataBuilder) {
	s.mu.Lock()
	s.metadataBuilder = builder
	logger.Debugf("%sconfig metadataBuilder is %#v", ServerLogPrefix, builder)
	s.mu.Unlock()
}

// ConfigAlpnHandler is used to set alpnHandler by zipper
func (s *Server) ConfigAlpnHandler(h func(string) error) {
	s.mu.Lock()
	s.alpnHandler = h
	logger.Debugf("%sconfig alpnHandler is %#v", ServerLogPrefix, h)
	s.mu.Unlock()
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
	if s.alpnHandler == nil {
		s.alpnHandler = func(proto string) error {
			logger.Infof("%sclient alpn proto is: %s", ServerLogPrefix, proto)
			return nil
		}
	}
}

func (s *Server) validateRouter() error {
	if s.router == nil {
		return errors.New("server's router is nil")
	}
	return nil
}

func (s *Server) validateMetadataBuilder() error {
	if s.metadataBuilder == nil {
		return errors.New("server's metadataBuilder is nil")
	}
	return nil
}

// Options returns the options of server.
func (s *Server) Options() ServerOptions {
	return s.opts
}

// Connector returns the connector of server.
func (s *Server) Connector() Connector {
	return s.connector
}

// SetBeforeHandlers set the before handlers of server.
func (s *Server) SetBeforeHandlers(handlers ...FrameHandler) {
	s.beforeHandlers = append(s.beforeHandlers, handlers...)
}

// SetAfterHandlers set the after handlers of server.
func (s *Server) SetAfterHandlers(handlers ...FrameHandler) {
	s.afterHandlers = append(s.afterHandlers, handlers...)
}

// SetConnectionCloseHandlers set the connection close handlers of server.
func (s *Server) SetConnectionCloseHandlers(handlers ...ConnectionHandler) {
	s.connectionCloseHandlers = append(s.connectionCloseHandlers, handlers...)
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

func authName(name string) string {
	if name == "" {
		return "empty"
	}

	return name
}

func (s *Server) doConnectionCloseHandlers(qconn quic.Connection) {
	defer s.wg.Done()
	logger.Debugf("%s🖤 [%s] quic connection closed", ServerLogPrefix, qconn.RemoteAddr())
	for _, h := range s.connectionCloseHandlers {
		h(qconn)
	}
}
