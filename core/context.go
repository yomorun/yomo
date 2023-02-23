package core

import (
	"context"
	"io"
	"net"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/core/yerr"
	"golang.org/x/exp/slog"
)

var ctxPool sync.Pool

// Context for YoMo Server.
// Context be generated after a client coming,
// And stores clientInfo and serverInfo according to client and server.
// Context's lifecycle equal to stream.
type Context struct {
	conn Connection
	// Stream is the long-lived connection between client and server.
	Stream ContextWriterCloser
	// Frame receives from client.
	Frame frame.Frame

	// mu protected
	mu sync.RWMutex
	// Keys stores the key/value pairs in context.
	// It is Lazy initialized.
	Keys map[string]any

	metadataBuilder metadata.Builder

	Logger *slog.Logger
}

// ConnectionInfoKey is the key that a Context returns ClientInfo for
const ConnectionInfoKey = "_yomo/connectioninfo"

// Set is used to store a new key/value pair exclusively for this context.
// It also lazy initializes  c.Keys if it was not used previously.
func (c *Context) Set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.Keys == nil {
		c.Keys = make(map[string]any)
	}

	c.Keys[key] = value
}

// Get returns the value for the given key, ie: (value, true).
// If the value does not exist it returns (nil, false)
func (c *Context) Get(key string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	value, ok := c.Keys[key]
	return value, ok
}

var _ context.Context = &Context{}

// Done returns nil (chan which will wait forever) when c.Stream.Context() has no Context.
func (c *Context) Done() <-chan struct{} { return c.Stream.Context().Done() }

// Deadline returns that there is no deadline (ok==false) when c.Stream has no Context.
func (c *Context) Deadline() (deadline time.Time, ok bool) { return c.Stream.Context().Deadline() }

// Err returns nil when c.Request has no Context.
func (c *Context) Err() error { return c.Stream.Context().Err() }

// Value returns the value associated with this context for key, or nil
// if no value is associated with key. Successive calls to Value with
// the same key returns the same result.
func (c *Context) Value(key any) any {
	if keyAsString, ok := key.(string); ok {
		if val, exists := c.Keys[keyAsString]; exists {
			return val
		}
	}
	// There always returns nil, because quic.Stream.Context is not be allowed modify.
	return c.Stream.Context().Value(key)
}

// newContext returns a yomo context,
// The context implements standard library `context.Context` interface,
// The lifecycle of Context is equal to stream's taht be passed in.
func newContext(conn Connection, stream ContextWriterCloser, mb metadata.Builder, logger *slog.Logger) (c *Context, err error) {
	v := ctxPool.Get()
	if v == nil {
		c = new(Context)
	} else {
		c = v.(*Context)
	}
	connectionFrame := &frame.ConnectionFrame{
		Name:            conn.Name(),
		ClientID:        conn.ClientID(),
		ClientType:      byte(conn.ClientType()),
		ObserveDataTags: conn.ObserveDataTags(),
		Metadata:        conn.Metadata().Encode(),
	}

	c.conn = conn
	c.Stream = stream
	c.metadataBuilder = mb

	md, err := c.metadataBuilder.Build(connectionFrame)

	c.Set(ConnectionInfoKey, &connection{
		name:       connectionFrame.Name,
		clientType: ClientType(connectionFrame.ClientType),
		metadata:   md,
		clientID:   connectionFrame.ClientID,
	})

	c.Logger = logger.With(
		"client_id", connectionFrame.ClientID,
		"client_type", ClientType(connectionFrame.ClientType).String(),
		"client_name", connectionFrame.Name,
	)

	return
}

// ConnectionInfo get connection info from Context.
func (c *Context) ConnectionInfo() (ConnectionInfo, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	v, ok := c.Keys[ConnectionInfoKey]
	if ok {
		return v.(ConnectionInfo), true
	}
	return nil, false
}

// WithFrame sets a frame to context.
func (c *Context) WithFrame(f frame.Frame) error {
	c.Frame = f

	return nil
}

// Clean cleans the Context,
// Context is not available after called Clean,
//
// Warining: do not use any Context api after Clean, It maybe cause an error.
func (c *Context) Clean() {
	c.reset()
	ctxPool.Put(c)
}

func (c *Context) reset() {
	c.conn = nil
	c.Stream = nil
	c.Frame = nil
	c.metadataBuilder = nil
	c.Logger = nil
	for k := range c.Keys {
		delete(c.Keys, k)
	}
}

// QuicConnCloser represents a quic.Connection that can be close,
// the quic.Connection don't accept stream in Context scope.
type QuicConnCloser interface {
	// LocalAddr returns the local address.
	LocalAddr() net.Addr
	// RemoteAddr returns the address of the peer.
	RemoteAddr() net.Addr
	// CloseWithError closes the connection with an error.
	// The error string will be sent to the peer.
	CloseWithError(quic.ApplicationErrorCode, string) error
	// Context returns a context that is cancelled when the connection is closed.
	Context() context.Context
}

// ContextWriterCloser is a writer that holds a Context.
type ContextWriterCloser interface {
	// TODO: DELETE the Reader.
	io.Reader
	// Write writes data to the stream.
	// Write can be made to time out and return a net.Error with Timeout() == true
	// after a fixed time limit; see SetDeadline and SetWriteDeadline.
	// If the stream was canceled by the peer, the error implements the StreamError
	// interface, and Canceled() == true.
	// If the connection was closed due to a timeout, the error satisfies
	// the net.Error interface, and Timeout() will be true.
	io.Writer
	// Close closes the write-direction of the stream, peer don't known the closing.
	// Future calls to Write are not permitted after calling Close.
	// It must not be called concurrently with Write.
	// It must not be called after calling CancelWrite.
	io.Closer
	// Context returns a context that is cancelled when the stream is closed.
	// According to quic.go implement, Context can't be nil.
	Context() context.Context
}

// CloseWithError closes the stream and cleans the context.
func (c *Context) CloseWithError(code yerr.ErrorCode, msg string) {
	c.Logger.Debug("conn context close", "err_code", code, "err_msg", msg)

	if c.Stream != nil {
		c.Stream.Close()
	}

	if c.conn != nil {
		c.conn.Close()
	}
}

// ConnID get quic connection id
func (c *Context) ConnID() string {
	if c.conn == nil {
		return ""
	}
	return c.conn.ClientID()
}
