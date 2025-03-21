package core

import (
	"log/slog"
	"sync"

	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
)

var ctxPool sync.Pool

// Context is context for frame handling.
// The lifespan of the Context should align with the lifespan of the frame.
type Context struct {
	// Connection is the connection used for reading and writing frames.
	Connection *Connection
	// Frame receives from client.
	Frame *frame.DataFrame
	// FrameMetadata is the merged metadata from the frame.
	FrameMetadata metadata.M
	// mu is used to protect Keys from concurrent read and write operations.
	mu sync.RWMutex
	// Keys stores the key/value pairs in context, It is Lazy initialized.
	Keys map[string]any
	// Using Logger to log in connection handler scope, Logger is frame-level logger.
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
// Returns (nil, false) if the value does not exist.
func (c *Context) Get(key string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	value, ok := c.Keys[key]
	return value, ok
}

// newContext returns a new YoMo context that implements the standard library `context.Context` interface.
// The YoMo context is used to manage the lifecycle of a connection and provides a way to pass data and metadata
// between connection processing functions. The lifecycle of the context is equal to the lifecycle of the connection
// that it is associated with. The context can be used to manage timeouts, cancellations, and other aspects of connection processing.
func newContext(conn *Connection, df *frame.DataFrame) (c *Context, err error) {
	fmd, err := metadata.Decode(df.Metadata)
	if err != nil {
		return nil, err
	}

	// compatible with client propagate target bug
	if df.Tag == ai.ReducerTag {
		SetMetadataTarget(fmd, "")
	}

	// merge connection metadata.
	conn.Metadata().Range(func(k, v string) bool {
		fmd.Set(k, v)
		return true
	})

	v := ctxPool.Get()
	if v == nil {
		c = new(Context)
	} else {
		c = v.(*Context)
	}

	c.Frame = df
	c.FrameMetadata = fmd

	c.Connection = conn

	// log with tid
	c.Logger = c.Connection.Logger.With("tid", GetTIDFromMetadata(fmd))

	return
}

// CloseWithError close connection with an error string.
func (c *Context) CloseWithError(errString string) {
	c.Logger.Debug("connection closed", "err", errString)

	err := c.Connection.FrameConn().CloseWithError(errString)
	if err == nil {
		return
	}
	c.Logger.Error("connection close failed", "err", err)
}

// Release release the Context, the Context which has been released will not be available.
//
// Warning: do not use any Context api after Release, It maybe cause an error.
func (c *Context) Release() {
	c.reset()
	ctxPool.Put(c)
}

func (c *Context) reset() {
	c.Connection = nil
	c.Frame = nil
	c.FrameMetadata = nil
	c.Logger = nil
	clear(c.Keys)
}
