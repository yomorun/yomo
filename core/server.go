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

// ErrServerClosed is returned by the Server's Serve and ListenAndServe methods after a call to Shutdown or Close.
var ErrServerClosed = errors.New("yomo: Server closed")

// FrameHandler is the handler for frame.
type FrameHandler func(c *Context) error

// ConnectionHandler is the handler for quic connection
type ConnectionHandler func(conn quic.Connection)

// Server is the underlining server of Zipper
type Server struct {
	ctx                     context.Context
	ctxCancel               context.CancelFunc
	name                    string
	connector               *Connector
	router                  router.Router
	metadataDecoder         metadata.Decoder
	counterOfDataFrame      int64
	downstreams             map[string]FrameWriterConnection
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

	logger := options.logger.With("component", "zipper", "zipper_name", name)

	ctx, ctxCancel := context.WithCancel(context.Background())

	s := &Server{
		ctx:         ctx,
		ctxCancel:   ctxCancel,
		name:        name,
		downstreams: make(map[string]FrameWriterConnection),
		logger:      logger,
		opts:        options,
	}

	return s
}

// ListenAndServe starts the server.
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}

	s.logger = s.logger.With("zipper_addr", addr)

	// connect to all downstreams.
	for addr, client := range s.downstreams {
		go client.Connect(ctx, addr)
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

	s.connector = NewConnector(ctx)

	// listen the address
	listener, err := newListener(conn, s.opts.tlsConfig, s.opts.quicConfig, s.logger)
	if err != nil {
		s.logger.Error("failed to listen on quic", err)
		return err
	}
	s.listener = listener

	s.logger.Info("zipper is up and running", "pid", os.Getpid(), "quic", listener.Versions(), "auth_name", s.authNames())

	defer s.close(s.connector, s.listener, s.router)

	for {
		conn, err := s.listener.Accept(s.ctx)
		if err != nil {
			if err == s.ctx.Err() {
				return ErrServerClosed
			}
			s.logger.Error("accepted an error when accepting a connection", err)
			return err
		}
		err = s.opts.alpnHandler(conn.ConnectionState().TLS.NegotiatedProtocol)
		if err != nil {
			conn.CloseWithError(quic.ApplicationErrorCode(yerr.ErrorCodeRejected), err.Error())
			continue
		}

		logger := s.logger.With("remote_addr", conn.RemoteAddr(), "local_addr", conn.LocalAddr())

		stream0, err := conn.AcceptStream(ctx)
		if err != nil {
			continue
		}

		controlStream := NewServerControlStream(conn, NewFrameStream(stream0), logger)

		// Auth accepts a AuthenticationFrame from client. The first frame from client must be
		// AuthenticationFrame, It returns true if auth successful otherwise return false.
		// It response to client a AuthenticationAckFrame.
		md, err := controlStream.VerifyAuthentication(s.handleAuthenticationFrame)
		if err != nil {
			continue
		}

		go func(qconn quic.Connection) {
			streamGroup := NewStreamGroup(ctx, md, controlStream, s.connector, s.metadataDecoder, s.router, logger)

			defer streamGroup.Wait()
			defer s.doConnectionCloseHandlers(qconn)
			defer logger.Debug("quic connection closed")

			select {
			case <-ctx.Done():
				return
			case <-s.runWithStreamGroup(streamGroup, logger):
			}
		}(conn)
	}
}

func (s *Server) runWithStreamGroup(group *StreamGroup, logger *slog.Logger) <-chan struct{} {
	done := make(chan struct{})

	go func() {
		if err := group.Run(s.handleStreamContext); err != nil {
			logger.Error("connection closed", err)
		}
		done <- struct{}{}
	}()

	return done
}

// Logger returns the logger of server.
func (s *Server) Logger() *slog.Logger { return s.logger }

// Close will shutdown the server.
func (s *Server) Close() error {
	s.ctxCancel()
	return nil
}

func (s *Server) close(connector *Connector, listener Listener, router router.Router) error {
	for _, ds := range s.downstreams {
		ds.Close()
	}
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
			c.Logger.Error("encountered an error in the start handler", err)
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
				c.CloseWithError(yerr.ErrorCodeClosed, "data stream has been closed")
				c.Logger.Info("data stream has been closed")
				break
			}
			if errors.Is(err, net.ErrClosed) {
				// if client close the connection, net.ErrClosed will be raise
				// by quic-go IdleTimeoutError after connection's KeepAlive config.
				c.Logger.Debug("data stream network error", "error", net.ErrClosed)
				c.CloseWithError(yerr.ErrorCodeClosed, net.ErrClosed.Error())
				break
			}
			// any error occurred, we should close the stream
			// after this, conn.AcceptStream() will raise the error
			c.CloseWithError(yerr.ErrorCodeUnknown, err.Error())
			c.Logger.Debug("connection closed")
			break
		}

		// add frame to context
		c.WithFrame(f)

		// before frame handlers
		for _, handler := range s.beforeHandlers {
			if err := handler(c); err != nil {
				c.Logger.Error("encountered an error in the before handler", err)
				c.CloseWithError(yerr.ErrorCodeBeforeHandler, err.Error())
				return
			}
		}
		// main handler
		if err := s.mainFrameHandler(c); err != nil {
			c.Logger.Error("encountered an error in the main handler", err)
			c.CloseWithError(yerr.ErrorCodeMainHandler, err.Error())
			return
		}
		// after frame handler
		for _, handler := range s.afterHandlers {
			if err := handler(c); err != nil {
				c.Logger.Error("encountered an error in the after handler", err)
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
			c.CloseWithError(yerr.ErrorCodeData, fmt.Sprintf("handle dataFrame err: %v", err))
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
		s.logger.Debug("authentication successful")
	} else {
		s.logger.Warn("authentication failed", "credential", f.AuthName())
	}

	return md, ok, nil
}

func (s *Server) handleDataFrame(c *Context) error {
	// counter +1
	atomic.AddInt64(&s.counterOfDataFrame, 1)

	from := c.DataStream

	f := c.Frame.(*frame.DataFrame)

	frameMetadata, err := s.metadataDecoder.Decode(f.GetMetaFrame().Metadata())
	if err != nil {
		return err
	}

	md := frameMetadata.Merge(c.DataStream.Metadata())

	// route
	route := s.router.Route(md)
	if route == nil {
		errString := "can't find sfn route"
		c.Logger.Warn(errString)
		return errors.New(errString)
	}

	// find stream function ids from the route.
	streamIDs := route.GetForwardRoutes(f.GetDataTag())

	c.Logger.Debug("sfn routing", "data_tag", f.GetDataTag(), "sfn_stream_ids", streamIDs, "connector", s.connector.GetSnapshot())

	for _, toID := range streamIDs {
		stream, ok, err := s.connector.Get(toID)
		if err != nil {
			continue
		}
		if !ok {
			c.Logger.Error("can't find forward stream", errors.New("route sfn error"), "forward_stream_id", toID)
			continue
		}

		c.Logger.Info(
			"routing data frame",
			"from_stream_name", from.Name(),
			"from_stream_id", from.ID(),
			"to_stream_name", stream.Name(),
			"to_stream_id", toID,
			"data_frame", f.String(),
		)

		// write data frame to stream
		if err := stream.WriteFrame(f); err != nil {
			c.Logger.Error("failed to write frame for routing data", err)
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
	sourceStreams, err := s.connector.GetSourceStreams(sourceID, tag)
	if err != nil {
		return err
	}
	for _, source := range sourceStreams {
		if source != nil {
			c.Logger.Info("backflow to source", "source_conn_id", sourceID, "back_flow_frame", f.String())
			if err := source.WriteFrame(bf); err != nil {
				c.Logger.Error("failed to write frame for backflow to the source", err)
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
	return atomic.LoadInt64(&s.counterOfDataFrame)
}

// Downstreams return all the downstream servers.
func (s *Server) Downstreams() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()

	snapshotOfDownstream := make(map[string]string, len(s.downstreams))
	for addr, client := range s.downstreams {
		snapshotOfDownstream[addr] = client.Name()
	}
	return snapshotOfDownstream
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
	s.logger.Debug("config metadata decoder")
	s.mu.Unlock()
}

// ConfigAlpnHandler is used to set alpnHandler by zipper
func (s *Server) ConfigAlpnHandler(h func(string) error) {
	s.mu.Lock()
	s.opts.alpnHandler = h
	s.logger.Debug("config alpn handler")
	s.mu.Unlock()
}

// AddDownstreamServer add a downstream server to this server. all the DataFrames will be
// dispatch to all the downstreams.
func (s *Server) AddDownstreamServer(addr string, c FrameWriterConnection) {
	s.mu.Lock()
	s.downstreams[addr] = c
	s.mu.Unlock()
}

// dispatch every DataFrames to all downstreams
func (s *Server) dispatchToDownstreams(c *Context) {
	stream := c.DataStream

	if stream.StreamType() == StreamTypeSource {
		f := c.Frame.(*frame.DataFrame)
		if f.IsBroadcast() {
			if f.GetMetaFrame().Metadata() == nil || len(f.GetMetaFrame().Metadata()) == 0 {
				byteMd, err := stream.Metadata().Encode()
				if err != nil {
					c.Logger.Error("failed to dispatch to downstream", err)
				}
				f.GetMetaFrame().SetMetadata(byteMd)
			}
			for streamID, ds := range s.downstreams {
				c.Logger.Info("dispatching to downstream", "dispatch_stream_id", streamID, "tid", f.TransactionID())
				ds.WriteFrame(f)
			}
		}
	}
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

// Name returns the name of server.
func (s *Server) Name() string { return s.name }

func (s *Server) doConnectionCloseHandlers(qconn quic.Connection) {
	for _, h := range s.connectionCloseHandlers {
		h(qconn)
	}
}
