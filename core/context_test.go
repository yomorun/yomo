package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
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

	connInfo := &connection{
		stream:     stream,
		clientID:   "xxxxx",
		clientType: ClientTypeSource,
		name:       "yomo",
		metadata:   &metadata.Default{},
	}
	c, err := newContext(connInfo, stream, metadata.DefaultBuilder(), logger)
	assert.NoError(t, err)

	c.Logger.Debug("hello")
	logdata, err := os.ReadFile(logpath)
	assert.NoError(t, err)
	assert.Equal(t, string(logdata), "level=DEBUG msg=hello client_id=xxxxx client_type=Source client_name=yomo\n")

	ctxConnInfo, ok := c.ConnectionInfo()
	assert.True(t, ok)
	assert.Equal(t, ctxConnInfo.Name(), connInfo.Name())
	assert.Equal(t, ctxConnInfo.ClientID(), connInfo.ClientID())
	assert.Equal(t, ctxConnInfo.ClientType(), connInfo.ClientType())
	assert.Equal(t, ctxConnInfo.Metadata(), connInfo.Metadata())

	metadata := []byte("moc-metadata")

	dataFrame := frame.NewDataFrame()
	dataFrame.GetMetaFrame().SetMetadata(metadata)

	if err := c.WithFrame(dataFrame); err != nil {
		assert.NoError(t, err)
		return
	}

	ctxConnInfo, ok = c.ConnectionInfo()
	assert.True(t, ok)
	assert.Equal(t, ctxConnInfo.Name(), connInfo.Name())
	assert.Equal(t, ctxConnInfo.ClientID(), connInfo.ClientID())
	assert.Equal(t, ctxConnInfo.ClientType(), connInfo.ClientType())
	assert.Equal(t, ctxConnInfo.Metadata(), connInfo.Metadata())

	c.Logger.Debug("logtwice")
	logdata, err = os.ReadFile(logpath)
	assert.NoError(t, err)
	assert.Equal(t, string(logdata), "level=DEBUG msg=hello client_id=xxxxx client_type=Source client_name=yomo\nlevel=DEBUG msg=logtwice client_id=xxxxx client_type=Source client_name=yomo\n")

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

	mb := metadata.DefaultBuilder()

	connInfo := &connection{
		stream:     stream,
		clientID:   "xxxxx",
		clientType: ClientTypeSource,
		name:       "yomo",
		metadata:   &metadata.Default{},
	}

	t.Run("Clean Context", func(t *testing.T) {
		var assertAfterClean = func(t *testing.T, c *Context) {
			assert.Nil(t, c.conn)
			assert.Nil(t, c.Stream)
			assert.Nil(t, c.Frame)
			assert.Equal(t, c.ConnID(), "")
			assert.Contains(t, []any{map[string]any(nil), map[string]any{}}, c.Keys)
		}
		c, err := newContext(connInfo, stream, mb, logger)
		assert.NoError(t, err)
		c.Clean()
		assertAfterClean(t, c)

		// new twice
		c, err = newContext(connInfo, stream, mb, logger)
		assert.NoError(t, err)
		c.Clean()
		c.Set("a", "b")
		c.Clean()
		assertAfterClean(t, c)
	})

	t.Run("normal Context", func(t *testing.T) {
		c, err := newContext(connInfo, stream, mb, logger)
		assert.NoError(t, err)
		assert.NoError(t, c.Err())
	})

	t.Run("Close Context", func(t *testing.T) {
		c, err := newContext(connInfo, stream, mb, logger)
		assert.NoError(t, err)
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
