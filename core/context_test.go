package core

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/lucas-clemente/quic-go"
	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/yerr"
	"github.com/yomorun/yomo/core/ylog"
	"golang.org/x/exp/slog"
)

func TestContext(t *testing.T) {
	sctx := context.Background()

	file, err := os.OpenFile(filepath.Join(t.TempDir(), "mock-quic-stream"), os.O_CREATE, os.ModeAppend)
	if err != nil {
		assert.NoError(t, err)
	}

	stream := &mockReaderCloser{sctx, file}

	logpath := filepath.Join(t.TempDir(), "context.log")

	logger := slog.New(ylog.NewHandlerFromConfig(ylog.Config{
		Output:      logpath,
		DisableTime: true,
	}))

	c := newContext(&mockConn{baseCtx: sctx, connID: "101.102.103.104:0"}, stream, logger)

	clientInfo := &ClientInfo{
		ID:       "xxxxx",
		Type:     byte(ClientTypeSource),
		Name:     "yomo",
		AuthName: "token",
		Metadata: nil,
	}

	handshakeFrame := frame.NewHandshakeFrame(
		clientInfo.Name,
		clientInfo.ID,
		clientInfo.Type,
		[]frame.Tag{frame.Tag('a')},
		clientInfo.AuthName,
		"key|value",
	)

	c = c.WithFrame(handshakeFrame)

	c.Logger.Debug("hello")
	logdata, err := os.ReadFile(logpath)
	assert.NoError(t, err)
	assert.Equal(t, string(logdata), "level=DEBUG msg=hello conn_id=101.102.103.104:0 client_id=xxxxx client_type=Source client_name=yomo auth_name=token\n")

	ctxCientInfo, ok := c.ClientInfo()
	assert.True(t, ok)
	assert.Equal(t, ctxCientInfo, clientInfo)

	metadata := []byte("moc-metadata")

	dataFrame := frame.NewDataFrame()
	dataFrame.GetMetaFrame().SetMetadata(metadata)

	c = c.WithFrame(dataFrame)

	ctxCientInfo, ok = c.ClientInfo()
	assert.True(t, ok)
	assert.Equal(t, ctxCientInfo, &ClientInfo{
		ID:       clientInfo.ID,
		Type:     clientInfo.Type,
		Name:     clientInfo.Name,
		AuthName: clientInfo.AuthName,
		Metadata: metadata,
	})

	c.Logger.Debug("logtwice")
	logdata, err = os.ReadFile(logpath)
	assert.NoError(t, err)
	assert.Equal(t, string(logdata), "level=DEBUG msg=hello conn_id=101.102.103.104:0 client_id=xxxxx client_type=Source client_name=yomo auth_name=token\nlevel=DEBUG msg=logtwice conn_id=101.102.103.104:0 client_id=xxxxx client_type=Source client_name=yomo auth_name=token\n")

	os.Remove(logpath)
}

func TestContextErr(t *testing.T) {
	sctx := context.Background()

	file, err := os.OpenFile(filepath.Join(t.TempDir(), "mock-quic-stream"), os.O_CREATE, os.ModeAppend)
	if err != nil {
		assert.NoError(t, err)
	}

	dctx, cancel := context.WithCancel(sctx)

	stream := &mockReaderCloser{dctx, file}

	logpath := filepath.Join(t.TempDir(), "context.log")

	logger := slog.New(ylog.NewHandlerFromConfig(ylog.Config{
		Output:      logpath,
		DisableTime: true,
	}))

	t.Run("Clean Context", func(t *testing.T) {
		var assertAfterClean = func(t *testing.T, c *Context) {
			assert.Nil(t, c.Conn)
			assert.Nil(t, c.Stream)
			assert.Nil(t, c.Frame)
			assert.Equal(t, c.connID, "")
			assert.Contains(t, []any{map[string]any(nil), map[string]any{}}, c.Keys)
		}
		c := newContext(&mockConn{baseCtx: sctx, connID: "101.102.103.104:0"}, stream, logger)
		c.Clean()
		assertAfterClean(t, c)

		// new twice
		c = newContext(&mockConn{baseCtx: sctx, connID: "101.102.103.104:0"}, stream, logger)
		c.Set("a", "b")
		c.Clean()
		assertAfterClean(t, c)
	})

	t.Run("normal Context", func(t *testing.T) {
		c := newContext(&mockConn{baseCtx: sctx, connID: "101.102.103.104:0"}, stream, logger)
		assert.NoError(t, c.Err())
	})

	t.Run("Close Context", func(t *testing.T) {
		c := newContext(&mockConn{baseCtx: sctx, connID: "101.102.103.104:0"}, stream, logger)
		c.CloseWithError(yerr.ErrorCodeClosed, "closed")

		cancel()

		done := <-c.Done()
		assert.Equal(t, done, struct{}{})

		assert.Equal(t, c.Err(), context.Canceled)
	})
}

var _ ContextWriterCloser = &mockReaderCloser{}

type mockReaderCloser struct {
	ctx  context.Context
	file *os.File
}

func (c *mockReaderCloser) Read(p []byte) (n int, err error)  { return c.file.Read(p) }
func (c *mockReaderCloser) Write(p []byte) (n int, err error) { return c.file.Write(p) }
func (c *mockReaderCloser) Close() error                      { return c.file.Close() }
func (c *mockReaderCloser) Context() context.Context          { return c.ctx }

var _ QuicConnCloser = &mockConn{}

type mockConn struct {
	mu sync.Mutex

	errorCode quic.ApplicationErrorCode
	msg       string

	connID  string
	baseCtx context.Context
}

// CloseWithError implements QuicConnCloser
func (c *mockConn) CloseWithError(code quic.ApplicationErrorCode, msg string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.errorCode = code
	c.msg = msg
	return nil
}
func (c *mockConn) Context() context.Context { return c.baseCtx }
func (c *mockConn) LocalAddr() net.Addr      { return &net.UDPAddr{IP: net.IPv4('a', 'b', 'c', 'd')} }
func (c *mockConn) RemoteAddr() net.Addr     { return mustResolveIPAddr(c.connID) }

func mustResolveIPAddr(connID string) net.Addr {
	addr, err := net.ResolveUDPAddr("udp", connID)
	if err != nil {
		panic(err)
	}
	return addr
}
