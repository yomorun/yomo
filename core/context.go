package core

import (
	"context"
	"sync"
	"time"

	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/router"
	"github.com/yomorun/yomo/core/yerr"
	"golang.org/x/exp/slog"
)

var ctxPool sync.Pool

// Context is context for stream handling.
// Context be generated after a dataStream coming, And stores some infomation
// from dataStream, Context's lifecycle is equal to stream's.
type Context struct {
	// DataStream is the stream used for reading and writing frames.
	DataStream DataStream

	// Frame receives from client.
	Frame frame.Frame

	// Route is the route from handshake.
	Route router.Route
	// mu is used to protect Keys from concurrent read and write operations.
	mu sync.RWMutex
	// Keys stores the key/value pairs in context, It is Lazy initialized.
	Keys map[string]any
	// Using Logger to log in stream handler scope.
	Logger *slog.Logger
}

// Set is used to store a new key/value pair exclusively for this context.
// It also lazy initializes c.Keys if it was not used previously.
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
// The lifecycle of Context is equal to stream's that be passed in.
func newContext(dataStream DataStream, route router.Route, logger *slog.Logger) (c *Context) {
	v := ctxPool.Get()
	if v == nil {
		c = new(Context)
	} else {
		c = v.(*Context)
	}

	c.Logger = logger.With(
		"stream_id", dataStream.ID(),
		"stream_name", dataStream.Name(),
		"stream_type", dataStream.StreamType().String(),
	)

	c.DataStream = dataStream
	c.Route = route
	c.Logger = logger

	return
}

// WithFrame sets a frame to context.
//
// TODO: delete frame from context due to different lifecycle between stream and stream.
func (c *Context) WithFrame(f frame.Frame) {
	c.Frame = f
}

// CloseWithError close dataStream in se error,
// It tells controlStream which dataStream should be closed and close dataStream with
// returning error message to client side stream.
//
// TODO: ycode is not be transmitted.
func (c *Context) CloseWithError(ycode yerr.ErrorCode, errString string) {
	c.Logger.Warn("data stream be closed", "err_code", ycode.String(), "error", errString)

	err := c.DataStream.Close()
	if err == nil {
		return
	}
	c.Logger.Error("data stream close failed", err)
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
	c.Route = nil
	c.Frame = nil
	c.Logger = nil
	for k := range c.Keys {
		delete(c.Keys, k)
	}
}

// StreamID gets dataStream ID.
func (c *Context) StreamID() string {
	return c.DataStream.ID()
}
