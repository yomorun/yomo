package core

import (
	"io"
	"sync"
	"time"

	"github.com/lucas-clemente/quic-go"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/yerr"
	"github.com/yomorun/yomo/pkg/logger"
)

// Context for YoMo Server.
type Context struct {
	// Conn is the connection of client.
	Conn   quic.Connection
	connID string
	// Stream is the long-lived connection between client and server.
	Stream io.ReadWriteCloser
	// Frame receives from client.
	Frame frame.Frame
	// Keys store the key/value pairs in context.
	Keys map[string]interface{}

	mu sync.RWMutex
}

func newContext(conn quic.Connection, stream quic.Stream) *Context {
	return &Context{
		Conn:   conn,
		connID: conn.RemoteAddr().String(),
		Stream: stream,
		// keys:    make(map[string]interface{}),
	}
}

// WithFrame sets a frame to context.
func (c *Context) WithFrame(f frame.Frame) *Context {
	c.Frame = f
	return c
}

// Clean the context.
func (c *Context) Clean() {
	logger.Debugf("%sconn[%s] context clean", ServerLogPrefix, c.connID)
	c.Stream = nil
	c.Frame = nil
	c.Keys = nil
	c.Conn = nil
}

// CloseWithError closes the stream and cleans the context.
func (c *Context) CloseWithError(code yerr.ErrorCode, msg string) {
	logger.Debugf("%sconn[%s] context close, errCode=%#x, msg=%s", ServerLogPrefix, c.connID, code, msg)
	if c.Stream != nil {
		c.Stream.Close()
	}
	if c.Conn != nil {
		c.Conn.CloseWithError(quic.ApplicationErrorCode(code), msg)
	}
	c.Clean()
}

// ConnID get quic connection id
func (c *Context) ConnID() string {
	return c.connID
}

// Set a key/value pair to context.
func (c *Context) Set(key string, value interface{}) {
	c.mu.Lock()
	if c.Keys == nil {
		c.Keys = make(map[string]interface{})
	}

	c.Keys[key] = value
	c.mu.Unlock()
}

// Get the value by a specified key.
func (c *Context) Get(key string) (value interface{}, exists bool) {
	c.mu.RLock()
	value, exists = c.Keys[key]
	c.mu.RUnlock()
	return
}

// GetString gets a string value by a specified key.
func (c *Context) GetString(key string) (s string) {
	if val, ok := c.Get(key); ok && val != nil {
		s, _ = val.(string)
	}
	return
}

// GetBool gets a bool value by a specified key.
func (c *Context) GetBool(key string) (b bool) {
	if val, ok := c.Get(key); ok && val != nil {
		b, _ = val.(bool)
	}
	return
}

// GetInt gets an int value by a specified key.
func (c *Context) GetInt(key string) (i int) {
	if val, ok := c.Get(key); ok && val != nil {
		i, _ = val.(int)
	}
	return
}

// GetInt64 gets an int64 value by a specified key.
func (c *Context) GetInt64(key string) (i64 int64) {
	if val, ok := c.Get(key); ok && val != nil {
		i64, _ = val.(int64)
	}
	return
}

// GetUint gets an uint value by a specified key.
func (c *Context) GetUint(key string) (ui uint) {
	if val, ok := c.Get(key); ok && val != nil {
		ui, _ = val.(uint)
	}
	return
}

// GetUint64 gets an uint64 value by a specified key.
func (c *Context) GetUint64(key string) (ui64 uint64) {
	if val, ok := c.Get(key); ok && val != nil {
		ui64, _ = val.(uint64)
	}
	return
}

// GetFloat64 gets a float64 value by a specified key.
func (c *Context) GetFloat64(key string) (f64 float64) {
	if val, ok := c.Get(key); ok && val != nil {
		f64, _ = val.(float64)
	}
	return
}

// GetTime gets a time.Time value by a specified key.
func (c *Context) GetTime(key string) (t time.Time) {
	if val, ok := c.Get(key); ok && val != nil {
		t, _ = val.(time.Time)
	}
	return
}

// GetDuration gets a time.Duration value by a specified key.
func (c *Context) GetDuration(key string) (d time.Duration) {
	if val, ok := c.Get(key); ok && val != nil {
		d, _ = val.(time.Duration)
	}
	return
}

// GetStringSlice gets a []string value by a specified key.
func (c *Context) GetStringSlice(key string) (ss []string) {
	if val, ok := c.Get(key); ok && val != nil {
		ss, _ = val.([]string)
	}
	return
}

// GetStringMap gets a map[string]interface{} value by a specified key.
func (c *Context) GetStringMap(key string) (sm map[string]interface{}) {
	if val, ok := c.Get(key); ok && val != nil {
		sm, _ = val.(map[string]interface{})
	}
	return
}

// GetStringMapString gets a map[string]string value by a specified key.
func (c *Context) GetStringMapString(key string) (sms map[string]string) {
	if val, ok := c.Get(key); ok && val != nil {
		sms, _ = val.(map[string]string)
	}
	return
}

// GetStringMapStringSlice gets a map[string][]string value by a specified key.
func (c *Context) GetStringMapStringSlice(key string) (smss map[string][]string) {
	if val, ok := c.Get(key); ok && val != nil {
		smss, _ = val.(map[string][]string)
	}
	return
}
