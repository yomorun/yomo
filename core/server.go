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
	"time"

	"github.com/lucas-clemente/quic-go"
	"github.com/yomorun/yomo/core/auth"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/core/router"
	"github.com/yomorun/yomo/core/yerr"
	"golang.org/x/exp/slog"

	// authentication implements, Currently, only token authentication is implemented
	_ "github.com/yomorun/yomo/pkg/auth"
)

// FrameHandler is the handler for frame.
type FrameHandler func(c *Context) error

// ConnectionHandler is the handler for quic connection
type ConnectionHandler func(conn quic.Connection)

// Server is the underlining server of Zipper
type Server struct {
	name                    string
	connector               Connector
	router                  router.Router
	metadataBuilder         metadata.Builder
	counterOfDataFrame      int64
	downstreams             map[string]frame.Writer
	mu                      sync.Mutex
	opts                    *serverOptions
	startHandlers           []FrameHandler
	beforeHandlers          []FrameHandler
	afterHandlers           []FrameHandler
	connectionCloseHandlers []ConnectionHandler
	listener                Listener
	wg                      *sync.WaitGroup
	logger                  *slog.Logger
}

// NewServer create a Server instance.
func NewServer(name string, opts ...ServerOption) *Server {
	options := defaultServerOptions()

	for _, o := range opts {
		o(options)
	}

	logger := options.logger.With("component", "server", "name", name)

	s := &Server{
		name:        name,
		connector:   newConnector(logger),
		downstreams: make(map[string]frame.Writer),
		wg:          new(sync.WaitGroup),
		logger:      logger,
		opts:        options,
	}

	return s
}

// ListenAndServe starts the server.
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	if addr == "" {
		addr = DefaultListenAddr
	}

	s.logger = s.logger.With("addr", addr)

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
	listener, err := newListener(conn, s.opts.tlsConfig, s.opts.quicConfig, s.logger)
	if err != nil {
		s.logger.Error("listener.Listen error", err)
		return err
	}
	s.listener = listener

	s.logger.Info("Listening", "pid", os.Getpid(), "quic", listener.Versions(), "auth_name", s.authNames())

	for {
		conn, err := s.listener.Accept(ctx)
		if err != nil {
			s.logger.Error("listener accept connections error", err)
			return err
		}
		err = s.opts.alpnHandler(conn.ConnectionState().TLS.NegotiatedProtocol)
		if err != nil {
			conn.CloseWithError(quic.ApplicationErrorCode(yerr.ErrorCodeRejected), err.Error())
			continue
		}

		// connection close handlers on server shutdown
		// defer s.doConnectionCloseHandlers(conn)
		s.wg.Add(1)
		connID := GetConnID(conn)
		s.logger.Info("new connection", "conn_id", connID)

		go func(qconn quic.Connection) {
			// connection close handlers on client connect timeout
			defer s.doConnectionCloseHandlers(qconn)
			for {
				s.logger.Debug("waiting for new stream")
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
					s.logger.Debug("close the connection", "client_name", name, "client_id", clientID, "conn_id", connID, "error", err)
					break
				}
				defer stream.Close()

				yctx, ok := s.handshakeWithTimeout(conn, stream, 10*time.Second)

				for _, handler := range s.startHandlers {
					if err := handler(yctx); err != nil {
						yctx.Logger.Error("startHandlers error", err)
						yctx.CloseWithError(yerr.ErrorCodeStartHandler, err.Error())
						return
					}
				}

				defer func() {
					if yctx != nil {
						yctx.Clean()
					}
				}()

				if !ok {
					return
				}

				s.logger.Info("stream created", "stream_id", stream.StreamID(), "conn_id", connID)

				s.handleConnection(yctx)
				yctx.Logger.Info("stream handleConnection DONE")
			}
		}(conn)
	}
}

// handshakeWithTimeout call handshake with a timeout.
func (s *Server) handshakeWithTimeout(conn quic.Connection, stream quic.Stream, timeout time.Duration) (*Context, bool) {
	type result struct {
		ok   bool
		yctx *Context
	}
	ch := make(chan result)

	fs := NewFrameStream(stream)

	go func() {
		ok, yctx := s.handshake(conn, stream, fs)
		ch <- result{yctx, ok}
	}()

	select {
	case <-time.After(timeout):
		return nil, false
	case r := <-ch:
		return r.yctx, r.ok
	}
}

// handshake accepts a handshakeFrame from client.
// the first frame from client must be handshakeFrame,
// It returns true if handshake successful otherwise return false.
// It response to client a handshakeAckFrame if the handshake is successful
// otherwise response a goawayFrame.
// It returns a context for this stream handler.
func (s *Server) handshake(conn quic.Connection, stream quic.Stream, fs frame.ReadWriter) (*Context, bool) {
	frm, err := fs.ReadFrame()

	c := newContext(conn, stream, s.metadataBuilder, s.logger)

	if err != nil {
		if err := fs.WriteFrame(frame.NewGoawayFrame(err.Error())); err != nil {
			s.logger.Error("write to client GoawayFrame error", err)
		}
		return c, false
	}

	if err := c.WithFrame(frm); err != nil {
		if err := fs.WriteFrame(frame.NewGoawayFrame(err.Error())); err != nil {
			s.logger.Error("write to client GoawayFrame error", err)
		}
		return c, false
	}

	if frm.Type() != frame.TagOfHandshakeFrame {
		c.Logger.Info("client not do handshake right off")
		if err := fs.WriteFrame(frame.NewGoawayFrame("handshake failed")); err != nil {
			s.logger.Error("write to client GoawayFrame error", err)
		}
		return c, false
	}

	if err := s.handleHandshakeFrame(c); err != nil {
		c.Logger.Info("handshake failed", "error", err)
		if err := fs.WriteFrame(frame.NewGoawayFrame(err.Error())); err != nil {
			s.logger.Error("write to client GoawayFrame error", err)
		}
		return c, false
	}

	return c, true
}

// Logger returns the logger of server.
func (s *Server) Logger() *slog.Logger { return s.logger }

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

// handleConnection handles streams on a connection,
// use c.Logger in this function scope for more complete logger information.
func (s *Server) handleConnection(c *Context) {
	fs := NewFrameStream(c.Stream)
	// check update for stream
	for {
		f, err := fs.ReadFrame()
		if err != nil {
			// if client close connection, will get ApplicationError with code = 0x00
			if e, ok := err.(*quic.ApplicationError); ok {
				if yerr.Is(e.ErrorCode, yerr.ErrorCodeClientAbort) {
					// client abort
					c.Logger.Info("client close the connection")
					break
				} else {
					ye := yerr.New(yerr.Parse(e.ErrorCode), err)
					c.Logger.Error("read frame error", ye)
				}
			} else if err == io.EOF {
				c.Logger.Info("connection EOF")
				break
			}
			if errors.Is(err, net.ErrClosed) {
				// if client close the connection, net.ErrClosed will be raise
				// by quic-go IdleTimeoutError after connection's KeepAlive config.
				c.Logger.Warn("connection error", "error", net.ErrClosed)
				c.CloseWithError(yerr.ErrorCodeClosed, "net.ErrClosed")
				break
			}
			// any error occurred, we should close the stream
			// after this, conn.AcceptStream() will raise the error
			c.CloseWithError(yerr.ErrorCodeUnknown, err.Error())
			c.Logger.Warn("connection close")
			break
		}

		// add frame to context
		if err := c.WithFrame(f); err != nil {
			c.CloseWithError(yerr.ErrorCodeGoaway, err.Error())
		}

		// before frame handlers
		for _, handler := range s.beforeHandlers {
			if err := handler(c); err != nil {
				c.Logger.Error("beforeFrameHandler error", err)
				c.CloseWithError(yerr.ErrorCodeBeforeHandler, err.Error())
				return
			}
		}
		// main handler
		if err := s.mainFrameHandler(c); err != nil {
			c.Logger.Error("mainFrameHandler error", err)
			c.CloseWithError(yerr.ErrorCodeMainHandler, err.Error())
			return
		}
		// after frame handler
		for _, handler := range s.afterHandlers {
			if err := handler(c); err != nil {
				c.Logger.Error("afterFrameHandler error", err)
				c.CloseWithError(yerr.ErrorCodeAfterHandler, err.Error())
				return
			}
		}
	}
}

func (s *Server) mainFrameHandler(c *Context) error {
	frameType := c.Frame.Type()

	switch frameType {
	case frame.TagOfHandshakeFrame:
		c.Logger.Warn("receive a handshakeFrame, ingonre it")
	case frame.TagOfDataFrame:
		if err := s.handleDataFrame(c); err != nil {
			c.CloseWithError(yerr.ErrorCodeData, fmt.Sprintf("handleDataFrame err: %v", err))
		} else {
			s.dispatchToDownstreams(c)

			// observe datatags backflow
			s.handleBackflowFrame(c)
		}
	default:
		c.Logger.Warn("unexpected frame", "unexpected_frame_type", frameType)
	}
	return nil
}

// handle HandShakeFrame
func (s *Server) handleHandshakeFrame(c *Context) error {
	f := c.Frame.(*frame.HandshakeFrame)

	// basic info
	connID := c.ConnID()
	clientID := f.ClientID
	clientType := ClientType(f.ClientType)
	stream := c.Stream
	// credential
	c.Logger.Debug("GOT HandshakeFrame", "client_type", f.ClientType, "client_id", clientID, "auth_name", authName(f.AuthName()))
	// authenticate
	authed := auth.Authenticate(s.opts.auths, f)
	c.Logger.Debug("authenticate", "authed", authed)
	if !authed {
		err := fmt.Errorf("handshake authentication fails, client credential name is %s", authName(f.AuthName()))
		// return err
		c.Logger.Debug("authenticated", "authed", authed)
		goawayFrame := frame.NewGoawayFrame(err.Error())
		if _, err = stream.Write(goawayFrame.Encode()); err != nil {
			c.Logger.Error("write to GoawayFrame failed", err, "authed", authed)
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
		conn = newConnection(f.Name, f.ClientID, clientType, metadata, stream, f.ObserveDataTags, c.Logger)

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
						c.Logger.Debug("write GoawayFrame", "error", e.Error(), "exists_conn_id", existsConnID)
						goawayFrame := frame.NewGoawayFrame(e.Error())
						if err := conn.WriteFrame(goawayFrame); err != nil {
							c.Logger.Error("write GoawayFrame failed", err)
							return err
						}
					}
				} else {
					return err
				}
			}
		}
	case ClientTypeUpstreamZipper:
		conn = newConnection(f.Name, f.ClientID, clientType, nil, stream, f.ObserveDataTags, c.Logger)
	default:
		// TODO: There is no need to Remove,
		// unknown client type is not be add to connector.
		s.connector.Remove(connID)
		err := fmt.Errorf("illegal ClientType: %#x", f.ClientType)
		c.CloseWithError(yerr.ErrorCodeUnknownClient, err.Error())
		return err
	}

	if _, err := stream.Write(frame.NewHandshakeAckFrame().Encode()); err != nil {
		c.Logger.Error("write handshakeAckFrame error", err)
	}

	s.connector.Add(connID, conn)
	c.Logger.Info("client is connected!")
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
		c.Logger.Warn("handleDataFrame connector cannot find", "from_conn_id", fromID)
		return fmt.Errorf("handleDataFrame connector cannot find %s", fromID)
	}

	f := c.Frame.(*frame.DataFrame)

	m, err := s.metadataBuilder.Decode(f.GetMetaFrame().Metadata())
	if err != nil {
		return err
	}
	metadata := m
	if metadata == nil {
		metadata = from.Metadata()
	}

	// route
	route := s.router.Route(metadata)
	if route == nil {
		c.Logger.Warn("handleDataFrame route is nil")
		return fmt.Errorf("handleDataFrame route is nil")
	}

	// get stream function connection ids from route
	connIDs := route.GetForwardRoutes(f.GetDataTag())
	for _, toID := range connIDs {
		conn := s.connector.Get(toID)
		if conn == nil {
			c.Logger.Error("Can't find forward conn", errors.New("conn is nil"), "forward_conn_id", toID)
			continue
		}

		to := conn.Name()
		c.Logger.Info(
			"handleDataFrame",
			"from_conn_name", from.Name(),
			"from_conn_id", fromID,
			"to_conn_name", to,
			"to_conn_id", toID,
			"data_frame", f.String(),
		)

		// write data frame to stream
		if err := conn.WriteFrame(f); err != nil {
			c.Logger.Error("handleDataFrame conn.Write", err)
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
	for _, source := range sourceConns {
		if source != nil {
			c.Logger.Info("handleBackflowFrame", "source_conn_id", sourceID, "back_flow_frame", f.String())
			if err := source.WriteFrame(bf); err != nil {
				c.Logger.Error("handleBackflowFrame conn.Write", err)
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
func (s *Server) Downstreams() map[string]frame.Writer {
	return s.downstreams
}

// ConfigRouter is used to set router by zipper
func (s *Server) ConfigRouter(router router.Router) {
	s.mu.Lock()
	s.router = router
	s.logger.Debug("config route")
	s.mu.Unlock()
}

// ConfigMetadataBuilder is used to set metadataBuilder by zipper
func (s *Server) ConfigMetadataBuilder(builder metadata.Builder) {
	s.mu.Lock()
	s.metadataBuilder = builder
	s.logger.Debug("config metadataBuilder")
	s.mu.Unlock()
}

// ConfigAlpnHandler is used to set alpnHandler by zipper
func (s *Server) ConfigAlpnHandler(h func(string) error) {
	s.mu.Lock()
	s.opts.alpnHandler = h
	s.logger.Debug("config alpnHandler")
	s.mu.Unlock()
}

// AddDownstreamServer add a downstream server to this server. all the DataFrames will be
// dispatch to all the downstreams.
func (s *Server) AddDownstreamServer(addr string, c frame.Writer) {
	s.mu.Lock()
	s.downstreams[addr] = c
	s.mu.Unlock()
}

// dispatch every DataFrames to all downstreams
func (s *Server) dispatchToDownstreams(c *Context) {
	conn := s.connector.Get(c.connID)
	if conn == nil {
		c.Logger.Debug("dispatchToDownstreams failed")
	} else if conn.ClientType() == ClientTypeSource {
		f := c.Frame.(*frame.DataFrame)
		if f.IsBroadcast() {
			if f.GetMetaFrame().Metadata() == nil {
				f.GetMetaFrame().SetMetadata(conn.Metadata().Encode())
			}
			for addr, ds := range s.downstreams {
				c.Logger.Info("dispatching to", "dispatch_addr", addr, "tid", f.TransactionID())
				ds.WriteFrame(f)
			}
		}
	}
}

// GetConnID get quic connection id
func GetConnID(conn quic.Connection) string {
	return conn.RemoteAddr().String()
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

// Connector returns the connector of server.
func (s *Server) Connector() Connector {
	return s.connector
}

// SetStartHandlers sets a function for operating connection,
// this function executes after handshake successful.
func (s *Server) SetStartHandlers(handlers ...FrameHandler) {
	s.startHandlers = append(s.startHandlers, handlers...)
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
	if len(s.opts.auths) == 0 {
		return []string{"none"}
	}
	result := []string{}
	for _, auth := range s.opts.auths {
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
	s.logger.Debug("quic connection closed")
	for _, h := range s.connectionCloseHandlers {
		h(qconn)
	}
}
