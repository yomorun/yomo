package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"reflect"
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
	"github.com/yomorun/yomo/pkg/frame-codec/y3codec"
	"github.com/yomorun/yomo/pkg/id"
	"github.com/yomorun/yomo/pkg/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// ErrServerClosed is returned by the Server's Serve and ListenAndServe methods after a call to Shutdown or Close.
var ErrServerClosed = errors.New("yomo: Server closed")

// FrameHandler is the handler for frame.
type FrameHandler func(c *Context) error

// ConnectionHandler is the handler for quic connection
type ConnectionHandler func(conn quic.Connection)

// Server is the underlying server of Zipper
type Server struct {
	ctx                     context.Context
	ctxCancel               context.CancelFunc
	name                    string
	connector               *Connector
	router                  router.Router
	codec                   frame.Codec
	packetReadWriter        frame.PacketReadWriter
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
	tracerProvider          oteltrace.TracerProvider
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
		ctx:              ctx,
		ctxCancel:        ctxCancel,
		name:             name,
		downstreams:      make(map[string]FrameWriterConnection),
		logger:           logger,
		tracerProvider:   options.tracerProvider,
		codec:            y3codec.Codec(),
		packetReadWriter: y3codec.PacketReadWriter(),
		opts:             options,
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
	if err := s.validateRouter(); err != nil {
		return err
	}

	s.connector = NewConnector(ctx)

	// listen the address
	listener, err := NewQuicListener(conn, s.opts.tlsConfig, s.opts.quicConfig, s.logger)
	if err != nil {
		s.logger.Error("failed to listen on quic", "err", err)
		return err
	}
	s.listener = listener

	s.logger.Info("zipper is up and running", "pid", os.Getpid(), "quic", s.opts.quicConfig.Versions, "auth_name", s.authNames())

	defer closeServer(s.downstreams, s.connector, s.listener, s.router)

	for {
		conn, err := s.listener.Accept(s.ctx)
		if err != nil {
			if err == s.ctx.Err() {
				return ErrServerClosed
			}
			s.logger.Error("accepted an error when accepting a connection", "err", err)
			return err
		}
		logger := s.logger.With("remote_addr", conn.RemoteAddr(), "local_addr", conn.LocalAddr())

		stream0, err := conn.AcceptStream(ctx)
		if err != nil {
			continue
		}

		controlStream := NewServerControlStream(conn, stream0, s.codec, s.packetReadWriter, logger)

		// Auth accepts a AuthenticationFrame from client. The first frame from client must be
		// AuthenticationFrame, It returns true if auth successful otherwise return false.
		// It response to client a AuthenticationAckFrame.
		md, err := controlStream.VerifyAuthentication(s.handleAuthenticationFrame)
		if err != nil {
			continue
		}

		go func(conn Connection) {
			streamGroup := NewStreamGroup(ctx, md, controlStream, s.connector, s.router, logger)

			defer streamGroup.Wait()
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
			logger.Error("connection closed", "err", err)
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

func closeServer(downstreams map[string]FrameWriterConnection, connector *Connector, listener Listener, router router.Router) error {
	for _, ds := range downstreams {
		ds.Close()
	}
	// connector
	if connector != nil {
		connector.Close()
	}
	// listener
	if listener != nil {
		listener.Close()
	}
	// router
	if router != nil {
		router.Clean()
	}
	return nil
}

// handleStreamContext handles data streams,
// use c.Logger in this function scope for more complete logger information.
func (s *Server) handleStreamContext(c *Context) {
	// start frame handlers
	for _, handler := range s.startHandlers {
		if err := handler(c); err != nil {
			c.Logger.Error("encountered an error in the start handler", "err", err)
			c.CloseWithError(err.Error())
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
				c.Logger.Error("read frame error", "err", ye)
			} else if err == io.EOF {
				c.CloseWithError("data stream has been closed")
				c.Logger.Info("data stream has been closed")
				break
			}
			if errors.Is(err, net.ErrClosed) {
				// if client close the connection, net.ErrClosed will be raise
				// by quic-go IdleTimeoutError after connection's KeepAlive config.
				c.Logger.Debug("data stream network error", "err", net.ErrClosed)
				c.CloseWithError(net.ErrClosed.Error())
				break
			}
			// any error occurred, we should close the stream
			// after this, conn.AcceptStream() will raise the error
			c.CloseWithError(err.Error())
			c.Logger.Debug("connection closed")
			break
		}

		// add frame to context
		if err := c.WithFrame(f); err != nil {
			c.CloseWithError(err.Error())
			break
		}

		// before frame handlers
		for _, handler := range s.beforeHandlers {
			if err := handler(c); err != nil {
				c.Logger.Error("encountered an error in the before handler", "err", err)
				c.CloseWithError(err.Error())
				return
			}
		}
		// main handler
		if err := s.mainFrameHandler(c); err != nil {
			c.Logger.Error("encountered an error in the main handler", "err", err)
			c.CloseWithError(err.Error())
			return
		}
		// after frame handler
		for _, handler := range s.afterHandlers {
			if err := handler(c); err != nil {
				c.Logger.Error("encountered an error in the after handler", "err", err)
				c.CloseWithError(err.Error())
				return
			}
		}
	}
}

func (s *Server) mainFrameHandler(c *Context) error {
	frameType := c.Frame.Type()

	switch frameType {
	case frame.TypeDataFrame:
		if err := s.handleDataFrame(c); err != nil {
			c.CloseWithError(fmt.Sprintf("handle dataFrame err: %v", err))
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

func (s *Server) handleAuthenticationFrame(f *frame.AuthenticationFrame) (metadata.M, bool, error) {
	md, ok := auth.Authenticate(s.opts.auths, f)

	if ok {
		s.logger.Debug("authentication successful", "credential", f.AuthName)
	} else {
		s.logger.Warn("authentication failed", "credential", f.AuthName)
	}

	return md, ok, nil
}

func (s *Server) handleDataFrame(c *Context) error {
	// counter +1
	atomic.AddInt64(&s.counterOfDataFrame, 1)

	from := c.DataStream
	tid := GetTIDFromMetadata(c.FrameMetadata)
	sid := GetSIDFromMetadata(c.FrameMetadata)
	parentTraced := GetTracedFromMetadata(c.FrameMetadata)
	traced := false
	// trace
	tp := s.TracerProvider()
	if tp != nil {
		// create span
		var span oteltrace.Span
		var err error
		// set parent span, if not traced, use empty string
		if parentTraced {
			span, err = trace.NewSpan(tp, "zipper", "handle DataFrame", tid, sid)
		} else {
			span, err = trace.NewSpan(tp, "zipper", "handle DataFrame", "", "")
		}
		if err != nil {
			s.logger.Error("zipper trace error", "err", err)
		} else {
			defer span.End()
			tid = span.SpanContext().TraceID().String()
			sid = span.SpanContext().SpanID().String()
			traced = true
		}
	}
	if tid == "" {
		s.logger.Debug("zipper create new tid")
		tid = id.TID()
	}
	if sid == "" {
		s.logger.Debug("zipper create new sid")
		sid = id.SID()
	}
	// reallocate metadata with new TID and SID
	SetTIDToMetadata(c.FrameMetadata, tid)
	SetSIDToMetadata(c.FrameMetadata, sid)
	SetTracedToMetadata(c.FrameMetadata, traced || parentTraced)
	md, err := c.FrameMetadata.Encode()
	if err != nil {
		s.logger.Error("encode metadata error", "err", err)
		return err
	}
	c.Frame.Metadata = md
	s.logger.Debug("zipper metadata", "tid", tid, "sid", sid, "parentTraced", parentTraced, "traced", traced, "frome_stream_name", from.Name())
	// route
	route := s.router.Route(c.FrameMetadata)
	if route == nil {
		errString := "can't find sfn route"
		c.Logger.Warn(errString)
		return errors.New(errString)
	}

	// find stream function ids from the route.
	streamIDs := route.GetForwardRoutes(c.Frame.Tag)

	c.Logger.Debug("sfn routing", "data_tag", c.Frame.Tag, "sfn_stream_ids", streamIDs, "connector", s.connector.Snapshot())

	for _, toID := range streamIDs {
		stream, ok, err := s.connector.Get(toID)
		if err != nil {
			continue
		}
		if !ok {
			c.Logger.Error("can't find forward stream", "err", "route sfn error", "forward_stream_id", toID)
			continue
		}

		c.Logger.Info(
			"routing data frame",
			"from_stream_name", from.Name(),
			"from_stream_id", from.ID(),
			"to_stream_name", stream.Name(),
			"to_stream_id", toID,
		)

		// write data frame to stream
		if err := stream.WriteFrame(c.Frame); err != nil {
			c.Logger.Error("failed to write frame for routing data", "err", err)
		}
	}

	return nil
}

func (s *Server) handleBackflowFrame(c *Context) error {
	sourceID := GetSourceIDFromMetadata(c.FrameMetadata)
	// write to source with BackflowFrame
	bf := &frame.BackflowFrame{
		Tag:      c.Frame.Tag,
		Carriage: c.Frame.Payload,
	}
	sourceStreams, err := s.connector.Find(sourceIDTagFindStreamFunc(sourceID, c.Frame.Tag))
	if err != nil {
		return err
	}
	for _, source := range sourceStreams {
		if source != nil {
			c.Logger.Info("backflow to source", "source_conn_id", sourceID)
			if err := source.WriteFrame(bf); err != nil {
				c.Logger.Error("failed to write frame for backflow to the source", "err", err)
				return err
			}
		}
	}
	return nil
}

// sourceIDTagFindStreamFunc creates a FindStreamFunc that finds a source type stream matching the specified sourceID and tag.
func sourceIDTagFindStreamFunc(sourceID string, tag frame.Tag) FindStreamFunc {
	return func(stream StreamInfo) bool {
		for _, v := range stream.ObserveDataTags() {
			if v == tag &&
				stream.StreamType() == StreamTypeSource &&
				stream.ID() == sourceID {
				return true
			}
		}
		return false
	}
}

// StatsFunctions returns the sfn stats of server.
func (s *Server) StatsFunctions() map[string]string {
	return s.connector.Snapshot()
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

// AddDownstreamServer add a downstream server to this server. all the DataFrames will be
// dispatch to all the downstreams.
func (s *Server) AddDownstreamServer(addr string, c FrameWriterConnection) {
	s.mu.Lock()
	s.downstreams[addr] = c
	s.mu.Unlock()
}

// dispatch every DataFrames to all downstreams
func (s *Server) dispatchToDownstreams(c *Context) {
	if c.DataStream.StreamType() == StreamTypeUpstreamZipper {
		// loop protection
		return
	}

	var (
		tid = GetTIDFromMetadata(c.FrameMetadata)
		sid = GetSIDFromMetadata(c.FrameMetadata)
	)
	mdBytes, err := c.FrameMetadata.Encode()
	if err != nil {
		c.Logger.Error("failed to dispatch to downstream", "err", err)
		return
	}
	c.Frame.Metadata = mdBytes

	for streamID, ds := range s.downstreams {
		c.Logger.Info("dispatching to downstream", "dispatch_stream_id", streamID, "tid", tid, "sid", sid)
		ds.WriteFrame(c.Frame)
	}
}

func (s *Server) validateRouter() error {
	if s.router == nil {
		return errors.New("server's router is nil")
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

// TracerProvider returns the tracer provider of server.
func (s *Server) TracerProvider() oteltrace.TracerProvider {
	if s.tracerProvider == nil {
		return nil
	}
	if reflect.ValueOf(s.tracerProvider).IsNil() {
		return nil
	}
	return s.tracerProvider
}
