package core

import (
	"context"
	"errors"
	"fmt"
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
	"golang.org/x/exp/slog"

	// authentication implements, Currently, only token authentication is implemented
	_ "github.com/yomorun/yomo/pkg/auth"
	"github.com/yomorun/yomo/pkg/frame-codec/y3codec"
	"github.com/yomorun/yomo/pkg/id"
	pkgtls "github.com/yomorun/yomo/pkg/tls"
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
	listener                *quic.Listener
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

	// connect to all downstreams.
	for addr, client := range s.downstreams {
		go client.Connect(ctx, addr)
	}

	return s.Serve(ctx, conn)
}

func (s *Server) handshake(qconn quic.Connection, fs *FrameStream) (bool, router.Route, Connection) {
	var gerr error

	defer func() {
		if gerr == nil {
			_ = fs.WriteFrame(&frame.HandshakeAckFrame{})
		} else {
			_ = fs.WriteFrame(&frame.RejectedFrame{Message: gerr.Error()})
		}
	}()

	first, err := fs.ReadFrame()
	if err != nil {
		gerr = err
		return false, nil, nil
	}

	switch first.Type() {
	case frame.TypeHandshakeFrame:
		hf := first.(*frame.HandshakeFrame)

		conn, err := s.handleHandshakeFrame(qconn, fs, hf)
		if err != nil {
			gerr = err
			return false, nil, conn
		}

		route, err := s.handleRoute(hf, conn.Metadata())
		if err != nil {
			gerr = err
		}
		return true, route, conn
	default:
		gerr = fmt.Errorf("yomo: handshake read unexpected frame, read: %s", first.Type().String())
		return false, nil, nil
	}
}

func (s *Server) handleConnection(qconn quic.Connection, fs *FrameStream, logger *slog.Logger) {
	ok, route, conn := s.handshake(qconn, fs)
	if !ok {
		logger.Error("handshake failed")
		return
	}

	logger = logger.With("conn_id", conn.ID(), "conn_name", conn.Name())
	logger.Info("client connected", "remote_addr", qconn.RemoteAddr().String(), "client_type", conn.ClientType().String())

	c := newContext(conn, route, logger)

	s.handleContext(c)
}

func (s *Server) handleContext(c *Context) {
	for _, h := range s.startHandlers {
		if err := h(c); err != nil {
			c.CloseWithError(err.Error())
		}
	}

	go s.handleFrames(c)
}

func (s *Server) handleFrames(c *Context) {
	defer func() {
		if c.Connection.ClientType() == ClientTypeStreamFunction {
			_ = c.Route.Remove(c.Connection.ID())
		}
		_ = s.connector.Remove(c.Connection.ID())
		c.Release()
	}()
	for {
		f, err := c.Connection.ReadFrame()
		if err != nil {
			c.Logger.Info("failed to read frame", "err", err)
			return
		}

		if err := c.WithFrame(f); err != nil {
			c.CloseWithError(err.Error())
			return
		}

		for _, h := range s.beforeHandlers {
			if err := h(c); err != nil {
				c.CloseWithError(err.Error())
				return
			}
		}

		switch f.Type() {
		case frame.TypeDataFrame:
			if err := s.mainFrameHandler(c); err != nil {
				c.Logger.Info("failed to handle data frame", "err", err)
				return
			}
		}

		for _, h := range s.afterHandlers {
			if err := h(c); err != nil {
				c.CloseWithError(err.Error())
				return
			}
		}
	}

}

func (s *Server) handleRoute(hf *frame.HandshakeFrame, md metadata.M) (router.Route, error) {
	if hf.ClientType != byte(ClientTypeStreamFunction) {
		return nil, nil
	}
	route := s.router.Route(md)
	if route == nil {
		return nil, errors.New("yomo: can't find route in handshake metadata")
	}
	err := route.Add(hf.ID, hf.ObserveDataTags)
	if err != nil {
		return nil, err
	}
	return route, nil
}

func (s *Server) handleHandshakeFrame(qconn quic.Connection, fs *FrameStream, hf *frame.HandshakeFrame) (Connection, error) {
	md, ok := auth.Authenticate(s.opts.auths, hf)

	if !ok {
		s.logger.Warn("authentication failed", "credential", hf.AuthName)
		return nil, fmt.Errorf("authentication failed: client credential name is %s", hf.AuthName)
	}

	conn := newConnection(hf.Name, hf.ID, ClientType(hf.ClientType), md, hf.ObserveDataTags, qconn, fs)

	return conn, s.connector.Store(hf.ID, conn)
}

// Serve the server with a net.PacketConn.
func (s *Server) Serve(ctx context.Context, conn net.PacketConn) error {
	if err := s.validateRouter(); err != nil {
		return err
	}

	s.connector = NewConnector(ctx)

	tlsConfig := s.opts.tlsConfig
	if tlsConfig == nil {
		tc, err := pkgtls.CreateServerTLSConfig(conn.LocalAddr().String())
		if err != nil {
			return err
		}
		tlsConfig = tc
	}

	// listen the address
	listener, err := quic.Listen(conn, tlsConfig, s.opts.quicConfig)
	if err != nil {
		s.logger.Error("failed to listen on quic", "err", err)
		return err
	}
	s.listener = listener

	s.logger.Info("zipper is up and running", "zipper_addr", s.listener.Addr().String(), "pid", os.Getpid(), "quic", s.opts.quicConfig.Versions, "auth_name", s.authNames())

	defer closeServer(s.downstreams, s.connector, s.listener, s.router)

	for {
		qconn, err := s.listener.Accept(s.ctx)
		if err != nil {
			if err == s.ctx.Err() {
				return ErrServerClosed
			}
			s.logger.Error("accepted an error when accepting a connection", "err", err)
			return err
		}

		stream, err := qconn.AcceptStream(ctx)
		if err != nil {
			continue
		}

		fs := NewFrameStream(stream, y3codec.Codec(), y3codec.PacketReadWriter())

		go s.handleConnection(qconn, fs, s.logger)
	}
}

// Logger returns the logger of server.
func (s *Server) Logger() *slog.Logger {
	return s.logger
}

// Close will shutdown the server.
func (s *Server) Close() error {
	s.ctxCancel()
	return nil
}

func closeServer(downstreams map[string]FrameWriterConnection, connector *Connector, listener *quic.Listener, router router.Router) error {
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

func (s *Server) handleDataFrame(c *Context) error {
	dataFrame := c.Frame.(*frame.DataFrame)
	data_length := len(dataFrame.Payload)

	// counter +1
	atomic.AddInt64(&s.counterOfDataFrame, 1)

	from := c.Connection
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
	dataFrame.Metadata = md
	s.logger.Debug("zipper metadata", "tid", tid, "sid", sid, "parentTraced", parentTraced, "traced", traced, "frome_stream_name", from.Name())
	// route
	route := s.router.Route(c.FrameMetadata)
	if route == nil {
		errString := "can't find sfn route"
		c.Logger.Warn(errString)
		return errors.New(errString)
	}

	// find stream function ids from the route.
	connIDs := route.GetForwardRoutes(dataFrame.Tag)
	if len(connIDs) == 0 {
		c.Logger.Info("no observed", "tag", dataFrame.Tag, "data_length", data_length)
	}
	c.Logger.Debug("connector snapshot", "tag", dataFrame.Tag, "sfn_conn_ids", connIDs, "connector", s.connector.Snapshot())

	for _, toID := range connIDs {
		stream, ok, err := s.connector.Get(toID)
		if err != nil {
			continue
		}
		if !ok {
			c.Logger.Error("can't find forward stream", "err", "route sfn error", "forward_stream_id", toID)
			continue
		}

		c.Logger.Info("data routing", "tid", tid, "sid", sid, "tag", dataFrame.Tag, "data_length", data_length, "to_id", toID, "to_name", stream.Name())

		// write data frame to stream
		if err := stream.WriteFrame(dataFrame); err != nil {
			c.Logger.Error("failed to write frame for routing data", "err", err)
		}
	}

	return nil
}

func (s *Server) handleBackflowFrame(c *Context) error {
	dataFrame := c.Frame.(*frame.DataFrame)

	sourceID := GetSourceIDFromMetadata(c.FrameMetadata)
	// write to source with BackflowFrame
	bf := &frame.BackflowFrame{
		Tag:      dataFrame.Tag,
		Carriage: dataFrame.Payload,
	}
	sourceStreams, err := s.connector.Find(sourceIDTagFindConnectionFunc(sourceID, dataFrame.Tag))
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

// sourceIDTagFindConnectionFunc creates a FindStreamFunc that finds a source type stream matching the specified sourceID and tag.
func sourceIDTagFindConnectionFunc(sourceID string, tag frame.Tag) FindConnectionFunc {
	return func(conn ConnectionInfo) bool {
		for _, v := range conn.ObserveDataTags() {
			if v == tag &&
				conn.ClientType() == ClientTypeSource &&
				conn.ID() == sourceID {
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
		snapshotOfDownstream[addr] = client.ClientID()
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
	dataFrame := c.Frame.(*frame.DataFrame)
	if c.Connection.ClientType() == ClientTypeUpstreamZipper {
		c.Logger.Debug("ignored client", "client_type", c.Connection.ClientType().String())
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
	dataFrame.Metadata = mdBytes

	for _, ds := range s.downstreams {
		c.Logger.Info(
			"dispatching to downstream",
			"tid", tid, "sid", sid, "tag", dataFrame.Tag, "data_length", len(dataFrame.Payload),
			"downstream_id", ds.ClientID(), "downstream_name", ds.Name(),
		)
		_ = ds.WriteFrame(dataFrame)
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
