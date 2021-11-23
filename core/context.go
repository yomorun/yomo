package core

import (
	"io"
	"sync"
	"time"

	"github.com/lucas-clemente/quic-go"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/pkg/logger"
)

type Context struct {
	// TODO: remove quic.Session which is only specified for quic?
	Session quic.Session
	Stream  io.ReadWriteCloser
	Frame   frame.Frame
	Keys    map[string]interface{}

	// SendDataBack is the callback function when the zipper needs to send the data back to the client's connection.
	// For example, the data needs to be sent back to the connections from WebSocket Bridge.
	SendDataBack func(f frame.Frame) error

	// OnClose is the callback function when the conn (or stream) is closed.
	OnClose func()

	mu sync.RWMutex
}

func newContext(session quic.Session, stream quic.Stream) *Context {
	return &Context{
		Session: session,
		Stream:  stream,
		// keys:    make(map[string]interface{}),
	}
}

func (c *Context) WithFrame(f frame.Frame) *Context {
	c.Frame = f
	return c
}

func (c *Context) Clean() {
	logger.Debugf("%sconn[%s] context clean", ServerLogPrefix, c.ConnID())
	c.Session = nil
	c.Stream = nil
	c.Frame = nil
	c.Keys = nil
}

func (c *Context) CloseWithError(code uint64, msg string) {
	logger.Debugf("%sconn[%s] context close, errCode=%d, msg=%s", ServerLogPrefix, c.ConnID(), code, msg)
	if c.Stream != nil {
		c.Stream.Close()
	}
	if c.Session != nil {
		c.Session.CloseWithError(quic.ApplicationErrorCode(code), msg)
	}
	if c.OnClose != nil {
		c.OnClose()
	}
	c.Clean()
}

func (c *Context) Set(key string, value interface{}) {
	c.mu.Lock()
	if c.Keys == nil {
		c.Keys = make(map[string]interface{})
	}

	c.Keys[key] = value
	c.mu.Unlock()
}

func (c *Context) Get(key string) (value interface{}, exists bool) {
	c.mu.RLock()
	value, exists = c.Keys[key]
	c.mu.RUnlock()
	return
}

func (c *Context) GetString(key string) (s string) {
	if val, ok := c.Get(key); ok && val != nil {
		s, _ = val.(string)
	}
	return
}

func (c *Context) GetBool(key string) (b bool) {
	if val, ok := c.Get(key); ok && val != nil {
		b, _ = val.(bool)
	}
	return
}

func (c *Context) GetInt(key string) (i int) {
	if val, ok := c.Get(key); ok && val != nil {
		i, _ = val.(int)
	}
	return
}

func (c *Context) GetInt64(key string) (i64 int64) {
	if val, ok := c.Get(key); ok && val != nil {
		i64, _ = val.(int64)
	}
	return
}

func (c *Context) GetUint(key string) (ui uint) {
	if val, ok := c.Get(key); ok && val != nil {
		ui, _ = val.(uint)
	}
	return
}

func (c *Context) GetUint64(key string) (ui64 uint64) {
	if val, ok := c.Get(key); ok && val != nil {
		ui64, _ = val.(uint64)
	}
	return
}

func (c *Context) GetFloat64(key string) (f64 float64) {
	if val, ok := c.Get(key); ok && val != nil {
		f64, _ = val.(float64)
	}
	return
}

func (c *Context) GetTime(key string) (t time.Time) {
	if val, ok := c.Get(key); ok && val != nil {
		t, _ = val.(time.Time)
	}
	return
}

func (c *Context) GetDuration(key string) (d time.Duration) {
	if val, ok := c.Get(key); ok && val != nil {
		d, _ = val.(time.Duration)
	}
	return
}

func (c *Context) GetStringSlice(key string) (ss []string) {
	if val, ok := c.Get(key); ok && val != nil {
		ss, _ = val.([]string)
	}
	return
}

func (c *Context) GetStringMap(key string) (sm map[string]interface{}) {
	if val, ok := c.Get(key); ok && val != nil {
		sm, _ = val.(map[string]interface{})
	}
	return
}

func (c *Context) GetStringMapString(key string) (sms map[string]string) {
	if val, ok := c.Get(key); ok && val != nil {
		sms, _ = val.(map[string]string)
	}
	return
}

func (c *Context) GetStringMapStringSlice(key string) (smss map[string][]string) {
	if val, ok := c.Get(key); ok && val != nil {
		smss, _ = val.(map[string][]string)
	}
	return
}

func (c *Context) ConnID() string {
	if c.Session != nil {
		return c.Session.RemoteAddr().String()
	}
	return ""
}
