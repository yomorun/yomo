package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sync"
	"sync/atomic"

	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core/auth"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/core/router"

	// authentication implements, Currently, only token authentication is implemented
	_ "github.com/yomorun/yomo/pkg/auth"

	"github.com/yomorun/yomo/pkg/frame-codec/y3codec"
	yquic "github.com/yomorun/yomo/pkg/listener/quic"
	pkgtls "github.com/yomorun/yomo/pkg/tls"
)

// ErrServerClosed is returned by the Server's Serve and ListenAndServe methods after a call to Shutdown or Close.
var ErrServerClosed = errors.New("yomo: Server closed")

type (
	// FrameHandler handles a frame.
	FrameHandler func(*Context)
	// FrameMiddleware is a middleware for frame handler.
	FrameMiddleware func(FrameHandler) FrameHandler
)

// RejectReservedTagMiddleware reject reserved tag
func RejectReservedTagMiddleware(next FrameHandler) FrameHandler {
	return func(c *Context) {
		if err := frame.IsReservedTag(c.Frame.Tag); err != nil {
			c.CloseWithError(err.Error())
			return
		}
		next(c)
	}
}

type (
	// ConnHandler handles a connection.
	ConnHandler func(*Connection)
	// ConnMiddleware is a middleware for connection handler.
	ConnMiddleware func(ConnHandler) ConnHandler
)

// Server is the underlying server of Zipper
type Server struct {
	ctx                  context.Context
	ctxCancel            context.CancelFunc
	name                 string
	connector            Connector
	router               router.Router
	codec                frame.Codec
	packetReadWriter     frame.PacketReadWriter
	counterOfDataFrame   int64
	downstreams          map[string]Downstream
	mu                   sync.Mutex
	opts                 *serverOptions
	frameHandler         FrameHandler
	connHandler          ConnHandler
	listener             frame.Listener
	logger               *slog.Logger
	versionNegotiateFunc VersionNegotiateFunc
}

// NewServer create a Server instance.
func NewServer(name string, opts ...ServerOption) *Server {
	options := defaultServerOptions()

	for _, o := range opts {
		o(options)
	}

	logger := options.logger.With("service", "zipper", "zipper_name", name)

	ctx, ctxCancel := context.WithCancel(context.Background())

	s := &Server{
		ctx:                  ctx,
		ctxCancel:            ctxCancel,
		name:                 name,
		downstreams:          make(map[string]Downstream),
		logger:               logger,
		connector:            options.connector,
		router:               options.router,
		codec:                y3codec.Codec(),
		packetReadWriter:     y3codec.PacketReadWriter(),
		opts:                 options,
		versionNegotiateFunc: options.versionNegotiateFunc,
	}

	if s.router == nil {
		s.router = router.Default()
	}
	if s.connector == nil {
		s.connector = NewConnector(ctx)
	}
	if s.versionNegotiateFunc == nil {
		s.versionNegotiateFunc = DefaultVersionNegotiateFunc
	}

	// work with middleware.
	s.connHandler = composeConnHandler(s.handleConn, s.opts.connMiddlewares...)
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

	listeners := append(s.opts.listeners, s.listener)

	var wg sync.WaitGroup
	for _, l := range listeners {
		wg.Add(1)
		go func(l frame.Listener) {
			errCount := 0
			for {
				fconn, err := l.Accept(s.ctx)
				if err != nil {
					if err == s.ctx.Err() {
						wg.Done()
						return
					}
					errCount++
					s.logger.Error("accepted an error when accepting a connection", "err", err, "err_count", errCount)
					continue
				}

				go s.handleFrameConn(fconn, s.logger)
			}
		}(l)
	}

	wg.Wait()
	return ErrServerClosed
}

func (s *Server) handleFrameConn(fconn frame.Conn, logger *slog.Logger) {
	conn, err := s.handshake(fconn)
	if err != nil {
		logger.Error("handshake failed", "err", err)
		return
	}

	// ack handshake
	_ = fconn.WriteFrame(&frame.HandshakeAckFrame{})

	s.connHandler(conn) // s.handleConn(conn) with middlewares

	if conn.ClientType() == ClientTypeStreamFunction {
		s.router.Remove(conn.ID())
		ai.UnregisterFunction(conn.ID(), conn.Metadata())
		s.logger.Info("unregister ai function", "conn_id", conn.ID())
	}
	_ = s.connector.Remove(conn.ID())
}

func rejectHandshake(w frame.Writer, err error) error {
	if err != nil {
		rf := &frame.RejectedFrame{
			Message: err.Error(),
		}
		_ = w.WriteFrame(rf)
	}

	return err
}

func connectToNewEndpoint(w frame.Writer, err *ErrConnectTo) error {
	if err == nil {
		return nil
	}
	cf := &frame.ConnectToFrame{
		Endpoint: err.Endpoint,
	}
	_ = w.WriteFrame(cf)

	return err
}

func (s *Server) handshake(fconn frame.Conn) (*Connection, error) {
	first, err := fconn.ReadFrame()
	if err != nil {
		return nil, err
	}

	switch first.Type() {
	case frame.TypeHandshakeFrame:

		hf := first.(*frame.HandshakeFrame)

		// 1. version negotiation
		if err := s.versionNegotiateFunc(hf.Version, Version); err != nil {
			if se := new(ErrConnectTo); errors.As(err, &se) {
				return nil, connectToNewEndpoint(fconn, se)
			}
			return nil, rejectHandshake(fconn, err)
		}

		// 2. authentication
		md, err := s.authenticate(hf)
		if err != nil {
			return nil, rejectHandshake(fconn, err)
		}

		// 3. try use function name as target
		tryUseFunctionNameAsTarget(hf)

		// 4. create connection
		conn, err := s.createConnection(hf, md, fconn)
		if err != nil {
			return nil, rejectHandshake(fconn, err)
		}

		// 5. try register function definition to global
		if err := s.tryRegisterFunctionDefinition(hf, conn, md); err != nil {
			return nil, rejectHandshake(fconn, err)
		}

		// 6. add route rules
		if err := s.addSfnRouteRule(conn.ID(), hf, conn.Metadata()); err != nil {
			return nil, rejectHandshake(fconn, err)
		}
		return conn, nil
	default:
		err = fmt.Errorf("yomo: handshake read unexpected frame, read: %s", first.Type().String())
		return nil, rejectHandshake(fconn, err)
	}
}

func tryUseFunctionNameAsTarget(hf *frame.HandshakeFrame) {
	if hf.ClientType != byte(ClientTypeStreamFunction) {
		return
	}
	if hf.FunctionDefinition != nil {
		hf.WantedTarget = hf.Name
		hf.ObserveDataTags = append(hf.ObserveDataTags, ai.FunctionCallTag)
	}
}

func (s *Server) tryRegisterFunctionDefinition(hf *frame.HandshakeFrame, conn *Connection, md metadata.M) error {
	definition := hf.FunctionDefinition
	if definition == nil {
		return nil
	}
	fd := ai.FunctionDefinition{}
	if err := json.Unmarshal([]byte(definition), &fd); err != nil {
		return fmt.Errorf("unmarshal function definition error: %s", err.Error())
	}
	if err := ai.RegisterFunction(&fd, conn.ID(), md); err != nil {
		return err
	}
	s.logger.Info("register ai function", "conn_id", conn.ID(), "function_name", fd.Name, "definition", string(definition))
	return nil
}

func (s *Server) handleConn(conn *Connection) {
	conn.Logger.Info("new client connected", "client_type", conn.ClientType().String())

	for {
		f, err := conn.FrameConn().ReadFrame()
		if err != nil {
			conn.Logger.Info("failed to read frame", "err", err)
			return
		}
		switch f.Type() {
		case frame.TypeDataFrame:
			c, err := newContext(conn, f.(*frame.DataFrame))
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

func (s *Server) authenticate(hf *frame.HandshakeFrame) (metadata.M, error) {
	md, err := auth.Authenticate(s.opts.auths, auth.DefaultAuth(), hf)
	if err != nil {
		s.logger.Error(
			"authentication failed",
			"err", err,
			"client_type", ClientType(hf.ClientType).String(),
			"client_name", hf.Name,
			"auth_name", hf.AuthName,
			"auth_payload", string(hf.AuthPayload),
		)
		return nil, err
	}

	return md, nil
}

func (s *Server) createConnection(hf *frame.HandshakeFrame, md metadata.M, fconn frame.Conn) (*Connection, error) {
	if hf.WantedTarget != "" {
		md.Set(metadata.WantedTargetKey, hf.WantedTarget)
	}
	conn := NewConnection(
		incrID(),
		hf.Name,
		hf.ID,
		ClientType(hf.ClientType),
		md,
		hf.ObserveDataTags,
		fconn,
		s.logger,
	)

	return conn, s.connector.Store(conn.ID(), conn)
}

func (s *Server) addSfnRouteRule(connID uint64, hf *frame.HandshakeFrame, md metadata.M) error {
	if hf.ClientType != byte(ClientTypeStreamFunction) {
		return nil
	}
	return s.router.Add(connID, hf.ObserveDataTags, md)
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
	dataLength := len(dataFrame.Payload)

	// counter +1
	atomic.AddInt64(&s.counterOfDataFrame, 1)

	mdBytes, err := c.FrameMetadata.Encode()
	if err != nil {
		c.Logger.Error("encode metadata error", "err", err)
		return err
	}
	dataFrame.Metadata = mdBytes

	// find stream function ids from the router.
	connIDs := s.router.Route(dataFrame.Tag, c.FrameMetadata)
	if len(connIDs) == 0 {
		c.Logger.Info("no observed", "tag", dataFrame.Tag, "data_length", dataLength)
	}
	c.Logger.Debug("connector snapshot", "tag", dataFrame.Tag, "sfn_conn_ids", connIDs, "connector", s.connector.Snapshot())

	for _, toID := range connIDs {
		conn, ok, err := s.connector.Get(toID)
		if err != nil {
			continue
		}
		if !ok {
			c.Logger.Error("can't find forward conn", "to_id", toID)
			continue
		}

		// write data frame to conn
		if err := conn.FrameConn().WriteFrame(dataFrame); err != nil {
			c.Logger.Error(
				"failed to route data", "err", err,
				"tag", dataFrame.Tag, "data_length", dataLength, "to_id", toID, "to_name", conn.Name(),
			)
		} else {
			c.Logger.Info(
				"data routing",
				"tag", dataFrame.Tag, "data_length", dataLength, "to_id", toID, "to_name", conn.Name(),
			)
		}
	}

	return nil
}

// dispatch every DataFrames to all downstreams
func (s *Server) dispatchToDownstreams(c *Context) error {
	dataFrame := &frame.DataFrame{
		Tag:      c.Frame.Tag,
		Payload:  c.Frame.Payload,
		Metadata: c.Frame.Metadata,
	}
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

func closeServer(downstreams map[string]Downstream, connector Connector, listener frame.Listener, router router.Router) error {
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
		router.Release()
	}
	return nil
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
