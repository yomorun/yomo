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

	"github.com/yomorun/yomo/core/auth"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/core/router"
	"golang.org/x/exp/slog"

	// authentication implements, Currently, only token authentication is implemented
	_ "github.com/yomorun/yomo/pkg/auth"
	"github.com/yomorun/yomo/pkg/frame-codec/y3codec"
	yquic "github.com/yomorun/yomo/pkg/listener/quic"
	pkgtls "github.com/yomorun/yomo/pkg/tls"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// ErrServerClosed is returned by the Server's Serve and ListenAndServe methods after a call to Shutdown or Close.
var ErrServerClosed = errors.New("yomo: Server closed")

type (
	// FrameHandler handles a frame.
	FrameHandler func(*Context)
	// FrameMiddleware is a middleware for frame handler.
	FrameMiddleware func(FrameHandler) FrameHandler
)

type (
	// ConnHandler handles a connection and route.
	ConnHandler func(*Connection, router.Route)
	// ConnMiddleware is a middleware for connection handler.
	ConnMiddleware func(ConnHandler) ConnHandler
)

// Server is the underlying server of Zipper
type Server struct {
	ctx                context.Context
	ctxCancel          context.CancelFunc
	name               string
	connector          *Connector
	router             router.Router
	codec              frame.Codec
	packetReadWriter   frame.PacketReadWriter
	counterOfDataFrame int64
	downstreams        map[string]Downstream
	mu                 sync.Mutex
	opts               *serverOptions
	frameHandler       FrameHandler
	connHandler        ConnHandler
	listener           frame.Listener
	logger             *slog.Logger
	tracerProvider     oteltrace.TracerProvider
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
		downstreams:      make(map[string]Downstream),
		logger:           logger,
		tracerProvider:   options.tracerProvider,
		codec:            y3codec.Codec(),
		packetReadWriter: y3codec.PacketReadWriter(),
		opts:             options,
	}

	// work with middleware.
	s.connHandler = composeConnHandler(s.handleConnRoute, s.opts.connMiddlewares...)
	s.frameHandler = composeFrameHandler(s.handleFrame, s.opts.frameMiddlewares...)

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
	for _, client := range s.downstreams {
		go client.Connect(ctx)
	}

	return s.Serve(ctx, conn)
}

// Serve the server with a net.PacketConn.
func (s *Server) Serve(ctx context.Context, conn net.PacketConn) error {
	if err := s.validateRouter(); err != nil {
		return err
	}

	s.connector = NewConnector(ctx)

	tlsConfig := s.opts.tlsConfig
	if tlsConfig == nil {
		tlsConfig = pkgtls.MustCreateServerTLSConfig(conn.LocalAddr().String())
	}

	// listen the address
	listener, err := yquic.Listen(conn, y3codec.Codec(), y3codec.PacketReadWriter(), tlsConfig, s.opts.quicConfig)
	if err != nil {
		s.logger.Error("failed to listen on quic", "err", err)
		return err
	}
	s.listener = listener

	s.logger.Info(
		"zipper is up and running",
		"zipper_addr", conn.LocalAddr().String(), "pid", os.Getpid(), "quic", s.opts.quicConfig.Versions, "auth_name", s.authNames())

	defer closeServer(s.downstreams, s.connector, s.listener, s.router)

	for {
		fconn, err := s.listener.Accept(s.ctx)
		if err != nil {
			if err == s.ctx.Err() {
				return ErrServerClosed
			}
			s.logger.Error("accepted an error when accepting a connection", "err", err)
			return err
		}

		go s.handleFrameConn(fconn, s.logger)
	}
}

func (s *Server) handshake(fconn frame.Conn) (bool, router.Route, *Connection) {
	var gerr error

	defer func() {
		if gerr == nil {
			_ = fconn.WriteFrame(&frame.HandshakeAckFrame{})
		} else {
			_ = fconn.WriteFrame(&frame.RejectedFrame{Message: gerr.Error()})
		}
	}()

	first, err := fconn.ReadFrame()
	if err != nil {
		gerr = err
		return false, nil, nil
	}
	switch first.Type() {
	case frame.TypeHandshakeFrame:
		hf := first.(*frame.HandshakeFrame)

		conn, err := s.handleHandshakeFrame(fconn, hf)
		if err != nil {
			gerr = err
			return false, nil, conn
		}

		route, err := s.addSfnToRoute(hf, conn.Metadata())
		if err != nil {
			gerr = err
		}
		return true, route, conn
	default:
		gerr = fmt.Errorf("yomo: handshake read unexpected frame, read: %s", first.Type().String())
		return false, nil, nil
	}
}

func (s *Server) handleConnRoute(conn *Connection, route router.Route) {
	conn.Logger.Info("new client connected", "client_type", conn.ClientType().String())

	for {
		f, err := conn.FrameConn().ReadFrame()
		if err != nil {
			conn.Logger.Info("failed to read frame", "err", err)
			return
		}
		switch f.Type() {
		case frame.TypeDataFrame:
			c, err := newContext(conn, route, f.(*frame.DataFrame))
			if err != nil {
				conn.Logger.Info("failed to new context", "err", err)
				return
			}

			s.frameHandler(c) // s.handleFrame(c) with middlewares

			c.Release()
		default:
			conn.Logger.Info("unexpected frame", "type", f.Type().String())
			return
		}
	}
}

func (s *Server) handleFrameConn(fconn frame.Conn, logger *slog.Logger) {
	ok, route, conn := s.handshake(fconn)
	if !ok {
		logger.Error("handshake failed")
		return
	}

	s.connHandler(conn, route) // s.handleConnRoute(conn, route) with middlewares

	if conn.ClientType() == ClientTypeStreamFunction {
		_ = route.Remove(conn.ID())
	}
	_ = s.connector.Remove(conn.ID())
}

func (s *Server) handleHandshakeFrame(fconn frame.Conn, hf *frame.HandshakeFrame) (*Connection, error) {
	md, ok := auth.Authenticate(s.opts.auths, hf)

	if !ok {
		s.logger.Warn(
			"authentication failed",
			"client_type", ClientType(hf.ClientType).String(), "client_name", hf.Name,
			"credential", hf.AuthName,
		)
		return nil, fmt.Errorf("authentication failed: client credential type is %s", hf.AuthName)
	}

	conn := newConnection(hf.Name, hf.ID, ClientType(hf.ClientType), md, hf.ObserveDataTags, fconn, s.logger)

	return conn, s.connector.Store(hf.ID, conn)
}

func (s *Server) addSfnToRoute(hf *frame.HandshakeFrame, md metadata.M) (router.Route, error) {
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

func (s *Server) handleFrame(c *Context) {
	// routing data frame.
	if err := s.routingDataFrame(c); err != nil {
		c.CloseWithError(fmt.Sprintf("handle dataFrame err: %v", err))
		return
	}

	// dispatch to downstream.
	if err := s.dispatchToDownstreams(c); err != nil {
		c.CloseWithError(fmt.Sprintf("dispatch to downstream err: %v", err))
		return
	}
}

func (s *Server) routingDataFrame(c *Context) error {
	dataFrame := c.Frame
	data_length := len(dataFrame.Payload)

	// counter +1
	atomic.AddInt64(&s.counterOfDataFrame, 1)

	md, endFn := ZipperTraceMetadata(c.FrameMetadata, s.TracerProvider(), c.Logger)
	defer endFn()

	c.FrameMetadata = md

	mdBytes, err := c.FrameMetadata.Encode()
	if err != nil {
		c.Logger.Error("encode metadata error", "err", err)
		return err
	}
	dataFrame.Metadata = mdBytes

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
		conn, ok, err := s.connector.Get(toID)
		if err != nil {
			continue
		}
		if !ok {
			c.Logger.Error("can't find forward conn", "to_id", toID, "to_name", conn.Name())
			continue
		}

		// write data frame to conn
		if err := conn.FrameConn().WriteFrame(dataFrame); err != nil {
			c.Logger.Error(
				"failed to route data", "err", err,
				"tag", dataFrame.Tag, "data_length", data_length, "to_id", toID, "to_name", conn.Name(),
			)
		} else {
			c.Logger.Info(
				"data routing",
				"tag", dataFrame.Tag, "data_length", data_length, "to_id", toID, "to_name", conn.Name(),
			)
		}
	}

	return nil
}

// dispatch every DataFrames to all downstreams
func (s *Server) dispatchToDownstreams(c *Context) error {
	dataFrame := c.Frame
	if c.Connection.ClientType() == ClientTypeUpstreamZipper {
		c.Logger.Debug("ignored client", "client_type", c.Connection.ClientType().String())
		// loop protection
		return nil
	}

	mdBytes, err := c.FrameMetadata.Encode()
	if err != nil {
		c.Logger.Error("failed to dispatch to downstream", "err", err)
		return err
	}
	dataFrame.Metadata = mdBytes

	for _, ds := range s.downstreams {

		if err = ds.WriteFrame(dataFrame); err != nil {
			c.Logger.Error(
				"failed to dispatch to downstream",
				"err", err,
				"tag", dataFrame.Tag, "data_length", len(dataFrame.Payload),
				"downstream_id", ds.ID(), "downstream_name", ds.LocalName(),
			)
		} else {
			c.Logger.Info(
				"dispatching to downstream",
				"tag", dataFrame.Tag, "data_length", len(dataFrame.Payload),
				"downstream_id", ds.ID(), "downstream_name", ds.LocalName(),
			)
		}
	}

	return nil
}

func closeServer(downstreams map[string]Downstream, connector *Connector, listener frame.Listener, router router.Router) error {
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
	for _, client := range s.downstreams {
		snapshotOfDownstream[client.LocalName()] = client.ID()
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
func (s *Server) AddDownstreamServer(c Downstream) {
	s.mu.Lock()
	s.downstreams[c.ID()] = c
	s.mu.Unlock()
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

func (s *Server) validateRouter() error {
	if s.router == nil {
		return errors.New("server's router is nil")
	}
	return nil
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

func composeFrameHandler(handler FrameHandler, middlewares ...FrameMiddleware) FrameHandler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}

func composeConnHandler(handler ConnHandler, middlewares ...ConnMiddleware) ConnHandler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}
