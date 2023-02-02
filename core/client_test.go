package core

import (
	"bytes"
	"context"
	"path/filepath"
	"runtime"
	"strconv"
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

func TestClientDialNothing(t *testing.T) {
	ctx := context.Background()

	client := NewClient("source", ClientTypeSource)

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
		ClientTypeSource,
		WithCredential("token:auth-token"),
		WithObserveDataTags(obversedTag),
		WithClientQuicConfig(DefalutQuicConfig),
		WithClientTLSConfig(nil),
		WithLogger(ylog.Default()),
	)

	source.SetBackflowFrameObserver(func(bf *frame.BackflowFrame) {
		assert.Equal(t, string(payload), string(bf.GetCarriage()))
	})

	err := source.Connect(ctx, testaddr)
	assert.NoError(t, err, "source connect must be success")
	assert.Equal(t, ConnStateConnected, source.State(), "source state should be ConnStateConnected")

	sfn := NewClient("sfn-1", ClientTypeStreamFunction, WithCredential("token:auth-token"), WithObserveDataTags(obversedTag))

	sfn.SetDataFrameObserver(func(bf *frame.DataFrame) {
		assert.Equal(t, string(payload), string(bf.GetCarriage()))
	})

	err = sfn.Connect(ctx, testaddr)
	assert.NoError(t, err, "sfn connect must be success")
	assert.Equal(t, ConnStateConnected, sfn.State(), "sfn state should be ConnStateConnected")

	// wait source and sfn handshake successful (not elegant).
	time.Sleep(time.Second)

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

	// TODO: closing server many times is blocking.
	assert.NoError(t, server.Close(), "server.Close() should not return error")

	assert.NoError(t, source.Close(), "source client.Close() should not return error")
	assert.NoError(t, sfn.Close(), "sfn client.Close() should not return error")
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
func BenchmarkDataFramePool(b *testing.B) {
	var (
		tag     = frame.Tag(0x15)
		payload = []byte("yomo")
	)

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			prev := frame.NewDataFrame()
			prev.SetCarriage(tag, payload)
			prev.SetBroadcast(true)

			// prev.Clean() // master 分支上未包含此方法
		}
	})
}

// go test -benchmem -run=^$ -bench ^Benchmark$ github.com/yomorun/yomo/core -v -cover -count=1
func Benchmark(t *testing.B) {
	runtime.GOMAXPROCS(1)

	t.Setenv("YOMO_LOG_OUTPUT", filepath.Join(t.TempDir(), "output.log"))
	t.Setenv("YOMO_LOG_ERROR_OUTPUT", filepath.Join(t.TempDir(), "erroutput.log"))

	var (
		obversedTag = frame.Tag(1)
		payload     = []byte("hello data frame")
	)

	i := 1024
	t.RunParallel(func(p *testing.PB) {
		for p.Next() {
			i++
			source := createTestSource(t, strconv.Itoa(i), obversedTag, payload)

			for j := 0; j < 10000; j++ {
				dataFrame := frame.NewDataFrame()
				dataFrame.SetSourceID(source.clientID)
				dataFrame.SetCarriage(obversedTag, []byte(strconv.Itoa(i)))
				dataFrame.SetBroadcast(true)

				source.WriteFrame(dataFrame)
				// dataFrame.Clean()
			}
			source.Close()
		}
	})
}

func createTestSource(t *testing.B, name string, obversedTag frame.Tag, payload []byte) *Client {
	source := NewClient(
		name,
		ClientTypeSource,
		WithObserveDataTags(obversedTag),
		WithClientQuicConfig(DefalutQuicConfig),
		WithLogger(ylog.Default()),
	)

	source.SetBackflowFrameObserver(func(bf *frame.BackflowFrame) {})

	err := source.Connect(context.TODO(), "127.0.0.1:9000")
	assert.NoError(t, err, "source connect must be success")
	assert.Equal(t, ConnStateConnected, source.State(), "source state should be ConnStateConnected")

	return source
}

// var wg sync.WaitGroup

// for i := 0; i < 100; i++ {
// 	wg.Add(1)
// 	go func(i int) {
// 		defer wg.Done()
// 		source := createTestSource(t, strconv.Itoa(i), obversedTag, payload)
// 		syncMap.Store(source, struct{}{})

// 		for j := 0; j < 10000; j++ {
// 			dataFrame := frame.NewDataFrame()
// 			dataFrame.SetSourceID(source.clientID)
// 			dataFrame.SetCarriage(obversedTag, []byte(strconv.Itoa(i)))
// 			dataFrame.SetBroadcast(true)

// 			source.WriteFrame(dataFrame)
// 			dataFrame.Clean()
// 		}
// 	}(i)
// }
// wg.Wait()
