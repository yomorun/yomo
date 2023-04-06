package core

import (
	"bytes"
	"context"
	"net"
	"sync"
	"testing"
	"time"

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
	err := client.Connect(ctx, testaddr)

	qerr := net.ErrClosed
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
	server.ConfigMetadataDecoder(metadata.DefaultDecoder())
	server.ConfigRouter(router.Default([]config.App{{Name: "sfn-1"}, {Name: "close-early-sfn"}}))

	// test server hooks
	ht := &hookTester{t}
	server.SetStartHandlers(ht.startHandler)
	server.SetBeforeHandlers(ht.beforeHandler)
	server.SetAfterHandlers(ht.afterHandler)

	recorder := newFrameWriterRecorder()
	server.AddDownstreamServer("mockAddr", recorder)

	go func() {
		server.ListenAndServe(ctx, testaddr)
	}()

	illegalTokenSource := NewClient("source", StreamTypeSource, WithCredential("token:error-token"), WithLogger(discardingLogger))
	err := illegalTokenSource.Connect(ctx, testaddr)
	assert.Equal(t, "yomo: authentication failed, client credential name is token", err.Error())

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

	err = source.Connect(ctx, testaddr)
	assert.NoError(t, err, "source connect must be success")
	closeEarlySfn := createTestStreamFunction("close-early-sfn", obversedTag)
	closeEarlySfn.Connect(ctx, testaddr)
	assert.Equal(t, nil, err)

	// test close early.
	closeEarlySfn.Close()
	assert.Equal(t, nil, err)

	sfn := createTestStreamFunction("sfn-1", obversedTag)
	err = sfn.Connect(ctx, testaddr)
	assert.NoError(t, err, "sfn connect must be success")

	// add a same name sfn to zipper.
	sameNameSfn := createTestStreamFunction("sfn-1", obversedTag)
	sameNameSfn.SetDataFrameObserver(func(bf *frame.DataFrame) {
		assert.Equal(t, string(payload), string(bf.GetCarriage()))
	})
	err = sameNameSfn.Connect(ctx, testaddr)
	assert.NoError(t, err, "sfn connect should replace the old sfn stream")

	exited := checkClientExited(sfn, time.Second)
	assert.True(t, exited, "the old sfn stream should exited")

	exited = checkClientExited(sameNameSfn, time.Second)
	assert.False(t, exited, "the new sfn stream should not exited")

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

	dataFrameEncoded := dataFrame.Encode()

	err = source.WriteFrame(dataFrame)
	assert.NoError(t, err, "source write dataFrame must be success")

	time.Sleep(time.Second)
	assert.Equal(t, recorder.frameBytes(), dataFrameEncoded)

	assert.NoError(t, source.Close(), "source client.Close() should not return error")
	assert.NoError(t, sfn.Close(), "sfn client.Close() should not return error")
	assert.NoError(t, server.Close(), "server.Close() should not return error")
}

func checkClientExited(client *Client, tim time.Duration) bool {
	done := make(chan struct{})
	go func() {
		client.Wait()
		done <- struct{}{}
	}()

	select {
	case <-done:
		return true
	case <-time.After(tim):
		return false
	}
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
		name,
		StreamTypeStreamFunction,
		WithCredential("token:auth-token"),
		WithObserveDataTags(obversedTag),
		WithLogger(discardingLogger),
	)
}

// frameWriterRecorder frames be writen.
type frameWriterRecorder struct {
	mu  sync.Mutex
	buf *bytes.Buffer
}

func newFrameWriterRecorder() *frameWriterRecorder {
	return &frameWriterRecorder{buf: bytes.NewBuffer([]byte{})}
}

func (w *frameWriterRecorder) WriteFrame(frm frame.Frame) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	_, err := w.buf.Write(frm.Encode())
	return err
}

func (w *frameWriterRecorder) frameBytes() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()

	bytes := w.buf.Bytes()
	return bytes
}
