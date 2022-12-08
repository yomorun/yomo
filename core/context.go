package core

import (
	"io"
	"sync"
	"time"

	"github.com/lucas-clemente/quic-go"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/yerr"
	"golang.org/x/exp/slog"
)

var ctxPool sync.Pool

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

	log *slog.Logger
}

func newContext(conn quic.Connection, stream quic.Stream, logger *slog.Logger) (ctx *Context) {
	v := ctxPool.Get()
	if v == nil {
		ctx = new(Context)
	} else {
		ctx = v.(*Context)
	}
	ctx.Conn = conn
	ctx.Stream = stream
	ctx.connID = conn.RemoteAddr().String()
	ctx.log = logger.With("conn_id", conn.RemoteAddr().String(), "stream_id", stream.StreamID())
	return
}

const clientInfoKey = "client_info"

type clientInfo struct {
	clientID   string
	clientType byte
	clientName string
	authName   string
}

// ClientInfo get client info from context.
func (c *Context) ClientInfo() *clientInfo {
	val, ok := c.Get(clientInfoKey)
	if !ok {
		return &clientInfo{}
	}
	return val.(*clientInfo)
}

// WithFrame sets a frame to context.
func (c *Context) WithFrame(f frame.Frame) *Context {
	if f.Type() == frame.TagOfHandshakeFrame {
		handshakeFrame := f.(*frame.HandshakeFrame)
		c.log.With(
			"client_id", handshakeFrame.ClientID,
			"client_type", ClientType(handshakeFrame.ClientType).String(),
			"client_name", handshakeFrame.Name,
			"auth_name", handshakeFrame.AuthName(),
		)
		c.Set(clientInfoKey, &clientInfo{
			clientID:   handshakeFrame.ClientID,
			clientType: handshakeFrame.ClientType,
			clientName: handshakeFrame.Name,
			authName:   handshakeFrame.AuthName(),
		})
	}
	c.log.With("frame_type", f.Type().String())
	c.Frame = f
	return c
}

// Clean the context.
func (c *Context) Clean() {
	c.log.Debug("conn context clean", "conn_id", c.connID)
	c.reset()
	ctxPool.Put(c)
}

func (c *Context) reset() {
	c.Conn = nil
	c.connID = ""
	c.Stream = nil
	c.Frame = nil
	c.log = nil
	for k := range c.Keys {
		delete(c.Keys, k)
	}
}

// CloseWithError closes the stream and cleans the context.
func (c *Context) CloseWithError(code yerr.ErrorCode, msg string) {
	c.log.Debug("conn context close, ", "err_code", code, "err_msg", msg)
	if c.Stream != nil {
		c.Stream.Close()
	}
	if c.Conn != nil {
		c.Conn.CloseWithError(quic.ApplicationErrorCode(code), msg)
	}
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
