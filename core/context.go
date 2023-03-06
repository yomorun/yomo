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
	"golang.org/x/exp/slog"
)

var ctxPool sync.Pool

// Context for YoMo Server.
// Context be generated after a dataStream coming, And stores some infomation
// from dataStream, the infomation cantains StreamInfo,
// Context's lifecycle is equal to stream's.
type Context struct {
	DataStream DataStream
	// Frame receives from client.
	Frame frame.Frame

	// mu protects Keys read write.
	mu sync.RWMutex
	// Keys stores the key/value pairs in context.
	// It is Lazy initialized.
	Keys map[string]any

	metadataBuilder metadata.Builder

	Logger *slog.Logger
}

// StreamInfoKey is the key that a Context returns StreamInfo
const StreamInfoKey = "_yomo/streaminfo"

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
func (c *Context) Done() <-chan struct{} { return c.DataStream.Context().Done() }

// Deadline returns that there is no deadline (ok==false) when c.Stream has no Context.
func (c *Context) Deadline() (deadline time.Time, ok bool) { return c.DataStream.Context().Deadline() }

// Err returns nil when c.Request has no Context.
func (c *Context) Err() error { return c.DataStream.Context().Err() }

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
	return c.DataStream.Context().Value(key)
}

// newContext returns a yomo context,
// The context implements standard library `context.Context` interface,
// The lifecycle of Context is equal to stream's taht be passed in.
func newContext(dataStream DataStream, mb metadata.Builder, logger *slog.Logger) (c *Context, err error) {
	v := ctxPool.Get()
	if v == nil {
		c = new(Context)
	} else {
		c = v.(*Context)
	}

	streamInfo := dataStream.StreamInfo()

	c.Set(StreamInfoKey, streamInfo)

	c.Logger = logger.With(
		"stream_id", streamInfo.ID(),
		"stream_name", streamInfo.Name(),
		"stream_type", streamInfo.StreamType().String(),
	)

	return
}

// StreamInfo get dataStream info from Context.
func (c *Context) StreamInfo() (StreamInfo, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	v, ok := c.Keys[StreamInfoKey]
	if ok {
		return v.(StreamInfo), true
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
	c.DataStream = nil
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

// StreamID gets dataStream ID.
func (c *Context) ConnID() string {
	if c.DataStream == nil {
		return ""
	}
	return c.DataStream.StreamInfo().ID()
}
