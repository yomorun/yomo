package core

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/core/router"
	"github.com/yomorun/yomo/core/ylog"
	"github.com/yomorun/yomo/pkg/config"
)

const testaddr = "127.0.0.1:19999"

var discardingLogger = ylog.NewFromConfig(ylog.Config{Output: "/dev/null", ErrorOutput: "/dev/null"})

// debugLogger be used to debug unittest.
// var debugLogger = ylog.NewFromConfig(ylog.Config{Verbose: true, Level: "debug"})

func TestClientDialNothing(t *testing.T) {
	ctx := context.Background()

	client := NewClient("source", StreamTypeSource, WithLogger(discardingLogger))

	assert.Equal(t, ConnStateReady, client.State(), "client state should be ConnStateReady")

	err := client.Connect(ctx, testaddr)

	assert.Equal(t, ConnStateDisconnected, client.State(), "client state should be ConnStateDisconnected")

	qerr := &quic.IdleTimeoutError{}
	assert.ErrorAs(t, err, &qerr, "dial must timeout")
}

func TestFrameRoundTrip(t *testing.T) {
	ctx := context.Background()

	var (
		obversedTag = frame.Tag(1)
		payload     = []byte("hello data frame")
	)

	server := NewServer("zipper",
		WithAddr(testaddr),
		WithAuth("token", "auth-token"),
		WithServerQuicConfig(DefalutQuicConfig),
		WithServerTLSConfig(nil),
		WithServerLogger(discardingLogger),
	)
	server.ConfigMetadataBuilder(metadata.DefaultBuilder())
	server.ConfigRouter(router.Default([]config.App{{Name: "sfn-1"}}))

	// test server hooks
	ht := &hookTester{t}
	server.SetStartHandlers(ht.startHandler)
	server.SetBeforeHandlers(ht.beforeHandler)
	server.SetAfterHandlers(ht.afterHandler)

	w := newMockFrameWriter()
	server.AddDownstreamServer("mockAddr", w)

	go func() {
		server.ListenAndServe(ctx, testaddr)
	}()

	source := NewClient(
		"source",
		StreamTypeSource,
		WithCredential("token:auth-token"),
		WithObserveDataTags(obversedTag),
		WithClientQuicConfig(DefalutQuicConfig),
		WithClientTLSConfig(nil),
		WithLogger(discardingLogger),
	)

	source.SetBackflowFrameObserver(func(bf *frame.BackflowFrame) {
		assert.Equal(t, string(payload), string(bf.GetCarriage()))
	})

	err := source.Connect(ctx, testaddr)
	assert.NoError(t, err, "source connect must be success")
	assert.Equal(t, ConnStateConnected, source.State(), "source state should be ConnStateConnected")

	sfn := createTestStreamFunction("sfn-1", obversedTag)

	sfn.SetDataFrameObserver(func(bf *frame.DataFrame) {
		assert.Equal(t, string(payload), string(bf.GetCarriage()))
	})

	err = sfn.Connect(ctx, testaddr)
	assert.NoError(t, err, "sfn connect must be success")
	assert.Equal(t, ConnStateConnected, sfn.State(), "sfn state should be ConnStateConnected")

	// add a same name sfn to zipper.
	sameNameSfn := createTestStreamFunction("sfn-1", obversedTag)
	err = sameNameSfn.Connect(ctx, testaddr)
	assert.Equal(t, err, nil)
	assert.Equal(t, ConnStateConnected, sameNameSfn.State(), "sfn state should be ConnStateConnected")

	time.Sleep(time.Second)
	assert.Equal(t, ConnStateClosed, sfn.State(), "sfn state should be ConnStateClosed after connecting same name")

	stats := server.StatsFunctions()
	nameList := []string{}
	for _, name := range stats {
		nameList = append(nameList, name)
	}
	assert.ElementsMatch(t, nameList, []string{"source", "sfn-1"})

	dataFrame := frame.NewDataFrame()
	dataFrame.SetSourceID(source.clientID)
	dataFrame.SetCarriage(obversedTag, payload)
	dataFrame.SetBroadcast(true)

	err = source.WriteFrame(dataFrame)
	assert.NoError(t, err, "source write dataFrame must be success")

	time.Sleep(time.Second)

	w.assertEqual(t, dataFrame)

	assert.NoError(t, source.Close(), "source client.Close() should not return error")
	assert.NoError(t, sfn.Close(), "sfn client.Close() should not return error")
	assert.NoError(t, server.Close(), "server.Close() should not return error")
}

type hookTester struct {
	t *testing.T
}

func (a *hookTester) startHandler(ctx *Context) error {
	ctx.Set("start", "yes")
	return nil
}

func (a *hookTester) beforeHandler(ctx *Context) error {
	ctx.Set("before", "ok")
	return nil
}

func (a *hookTester) afterHandler(ctx *Context) error {
	v, ok := ctx.Get("start")
	assert.True(a.t, ok)
	assert.Equal(a.t, v, "yes")

	v = ctx.Value("before")
	assert.True(a.t, ok)
	assert.Equal(a.t, v, "ok")

	return nil
}

func createTestStreamFunction(name string, obversedTag frame.Tag) *Client {
	return NewClient(
		"sfn-1",
		StreamTypeStreamFunction,
		WithCredential("token:auth-token"),
		WithObserveDataTags(obversedTag),
		WithLogger(discardingLogger),
	)
}

// mockFrameWriter mock a FrameWriter
type mockFrameWriter struct {
	mu  sync.Mutex
	buf *bytes.Buffer
}

func newMockFrameWriter() *mockFrameWriter { return &mockFrameWriter{buf: bytes.NewBuffer([]byte{})} }

func (w *mockFrameWriter) WriteFrame(frm frame.Frame) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	_, err := w.buf.Write(frm.Encode())
	return err
}

func (w *mockFrameWriter) assertEqual(t *testing.T, frm frame.Frame) {
	w.mu.Lock()
	defer w.mu.Unlock()

	assert.Equal(t, w.buf.Bytes(), frm.Encode())
}
