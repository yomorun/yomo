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

	"github.com/quic-go/quic-go"
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
	connector               *Connector
	router                  router.Router
	metadataDecoder         metadata.Decoder
	counterOfDataFrame      int64
	downstreams             map[string]frame.Writer
	mu                      sync.Mutex
	opts                    *serverOptions
	startHandlers           []FrameHandler
	beforeHandlers          []FrameHandler
	afterHandlers           []FrameHandler
	connectionCloseHandlers []ConnectionHandler
	listener                Listener
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
		downstreams: make(map[string]frame.Writer),
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
	if err := s.validateMetadataDecoder(); err != nil {
		return err
	}

	if err := s.validateRouter(); err != nil {
		return err
	}

	s.connector = NewConnector(ctx, s.logger)

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

		logger := s.logger.With("client_addr", conn.RemoteAddr())

		stream0, err := conn.AcceptStream(ctx)
		if err != nil {
			continue
		}

		controlStream := NewServerControlStream(conn, NewFrameStream(stream0), s.logger)

		// Auth accepts a AuthenticationFrame from client. The first frame from client must be
		// AuthenticationFrame, It returns true if auth successful otherwise return false.
		// It response to client a AuthenticationAckFrame.
		md, err := controlStream.VerifyAuthentication(s.handleAuthenticationFrame)
		if err != nil {
			logger.Error("Authentication Failed", err)
			continue
		}
		logger.Debug("Authentication Success")

		go func(qconn quic.Connection) {
			streamGroup := NewStreamGroup(ctx, md, controlStream, s.connector, s.metadataDecoder, s.router, logger)

			defer streamGroup.Wait()
			defer s.doConnectionCloseHandlers(qconn)

			select {
			case <-ctx.Done():
				return
			case err := <-s.runWithStreamGroup(streamGroup):
				logger.Error("client closed", err)
			}
		}(conn)
	}
}

func (s *Server) runWithStreamGroup(group *StreamGroup) <-chan error {
	errch := make(chan error)

	go func() {
		errch <- group.Run(s.handleStreamContext)
	}()

	return errch
}

// Logger returns the logger of server.
func (s *Server) Logger() *slog.Logger { return s.logger }

// Close will shutdown the server.
func (s *Server) Close() error {
	// connector
	if s.connector != nil {
		s.connector.Close()
	}
	// listener
	if s.listener != nil {
		s.listener.Close()
	}
	// router
	if s.router != nil {
		s.router.Clean()
	}
	return nil
}

// handleStreamContext handles data streams,
// use c.Logger in this function scope for more complete logger information.
func (s *Server) handleStreamContext(c *Context) {
	// start frame handlers
	for _, handler := range s.startHandlers {
		if err := handler(c); err != nil {
			c.Logger.Error("startHandlers error", err)
			c.CloseWithError(yerr.ErrorCodeStartHandler, err.Error())
			return
		}
	}

	// check update for stream
	for {
		f, err := c.DataStream.ReadFrame()
		if err != nil {
			// if client close connection, will get ApplicationError with code = 0x00
			if e, ok := err.(*quic.ApplicationError); ok {
				if yerr.Is(e.ErrorCode, yerr.ErrorCodeClientAbort) {
					// client abort
					c.Logger.Info("client close the connection")
					break
				}
				ye := yerr.New(yerr.Parse(e.ErrorCode), err)
				c.Logger.Error("read frame error", ye)
			} else if err == io.EOF {
				c.CloseWithError(yerr.ErrorCodeClosed, "data stream closed")
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
		c.WithFrame(f)

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
	case frame.TagOfDataFrame:
		if err := s.handleDataFrame(c); err != nil {
			c.CloseWithError(yerr.ErrorCodeData, fmt.Sprintf("handleDataFrame err: %v", err))
		} else {
			s.dispatchToDownstreams(c)

			// observe datatags backflow
			s.handleBackflowFrame(c)
		}
	default:
		c.Logger.Warn("unexpected frame", "unexpected_frame_type", frameType.String())
	}
	return nil
}

func (s *Server) handleAuthenticationFrame(f auth.Object) (metadata.Metadata, bool, error) {
	md, ok := auth.Authenticate(s.opts.auths, f)

	if ok {
		s.logger.Debug("Successful authentication")
	} else {
		s.logger.Warn("Authentication failed", "credential", f.AuthName())
	}

	return md, ok, nil
}

func (s *Server) handleDataFrame(c *Context) error {
	// counter +1
	atomic.AddInt64(&s.counterOfDataFrame, 1)

	fromID := c.StreamID()
	from, ok, err := s.connector.Get(fromID)
	if err != nil {
		return err
	}
	if !ok {
		c.Logger.Warn("handleDataFrame connector cannot find", "from_conn_id", fromID)
		return fmt.Errorf("handleDataFrame connector cannot find %s", fromID)
	}

	f := c.Frame.(*frame.DataFrame)

	frameMetadata, err := s.metadataDecoder.Decode(f.GetMetaFrame().Metadata())
	if err != nil {
		return err
	}

	md := frameMetadata.Merge(c.Metadata)

	// route
	route := s.router.Route(md)
	if route == nil {
		c.Logger.Warn("handleDataFrame route is nil")
		return fmt.Errorf("handleDataFrame route is nil")
	}

	// get stream function connection ids from route
	connIDs := route.GetForwardRoutes(f.GetDataTag())

	c.Logger.Debug(
		"Data Routing Status",
		"sfn_stream_ids", connIDs,
		"connector", s.connector.GetSnapshot(),
	)

	for _, toID := range connIDs {
		conn, ok, err := s.connector.Get(toID)
		if err != nil {
			continue
		}
		if !ok {
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
	sourceConns, err := s.connector.GetSourceStreams(sourceID, tag)
	if err != nil {
		return err
	}
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

// ConfigMetadataDecoder is used to set Decoder by zipper.
func (s *Server) ConfigMetadataDecoder(decoder metadata.Decoder) {
	s.mu.Lock()
	s.metadataDecoder = decoder
	s.logger.Debug("config metadataDecoder")
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
	stream, ok, err := s.connector.Get(c.StreamID())
	if err != nil {
		c.Logger.Error("Connector Get Error", err)
		return
	}
	if !ok {
		c.Logger.Debug("dispatchToDownstreams failed")
		return
	}

	if stream.StreamType() == StreamTypeSource {
		f := c.Frame.(*frame.DataFrame)
		if f.IsBroadcast() {
			if f.GetMetaFrame().Metadata() == nil || len(f.GetMetaFrame().Metadata()) == 0 {
				byteMd, err := stream.Metadata().Encode()
				if err != nil {
					c.Logger.Error("dispatching error", err)
				}
				f.GetMetaFrame().SetMetadata(byteMd)
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

func (s *Server) validateMetadataDecoder() error {
	if s.metadataDecoder == nil {
		return errors.New("server's metadataDecoder is nil")
	}
	return nil
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

func (s *Server) doConnectionCloseHandlers(qconn quic.Connection) {
	s.logger.Debug("QUIC Connection Closed")
	for _, h := range s.connectionCloseHandlers {
		h(qconn)
	}
}
