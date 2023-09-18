package core

import (
	"bytes"
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/core/router"
	"github.com/yomorun/yomo/core/ylog"
	"github.com/yomorun/yomo/pkg/config"
	"github.com/yomorun/yomo/pkg/frame-codec/y3codec"
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
		observedTag = frame.Tag(1)
		backflowTag = frame.Tag(2)
		payload     = []byte("hello data frame")
		backflow    = []byte("hello backflow frame")
	)

	server := NewServer("zipper",
		WithAuth("token", "auth-token"),
		WithServerQuicConfig(DefalutQuicConfig),
		WithServerTLSConfig(nil),
		WithServerLogger(discardingLogger),
	)
	server.ConfigRouter(router.Default([]config.Function{{Name: "sfn-1"}, {Name: "close-early-sfn"}}))

	// test server hooks
	ht := &hookTester{t}
	server.SetStartHandlers(ht.startHandler)
	server.SetBeforeHandlers(ht.beforeHandler)
	server.SetAfterHandlers(ht.afterHandler)

	recorder := newFrameWriterRecorder("mockClient")
	server.AddDownstreamServer("mockAddr", recorder)

	assert.Equal(t, map[string]string{"mockAddr": "mockClient"}, server.Downstreams())

	go func() {
		server.ListenAndServe(ctx, testaddr)
	}()

	illegalTokenSource := NewClient("source", StreamTypeSource, WithCredential("token:error-token"), WithLogger(discardingLogger))
	err := illegalTokenSource.Connect(ctx, testaddr)
	assert.Equal(t, "authentication failed: client credential name is token", err.Error())

	source := NewClient(
		"source",
		StreamTypeSource,
		WithCredential("token:auth-token"),
		WithClientQuicConfig(DefalutQuicConfig),
		WithClientTLSConfig(nil),
		WithLogger(discardingLogger),
		WithObserveDataTags(backflowTag),
		WithConnectUntilSucceed(),
		WithNonBlockWrite(),
	)

	source.SetBackflowFrameObserver(func(bf *frame.BackflowFrame) {
		assert.Equal(t, string(backflow), string(bf.Carriage))
	})

	err = source.Connect(ctx, testaddr)
	assert.NoError(t, err, "source connect must be success")
	closeEarlySfn := createTestStreamFunction("close-early-sfn", observedTag)
	closeEarlySfn.Connect(ctx, testaddr)
	assert.Equal(t, nil, err)

	// test close early.
	closeEarlySfn.Close()
	assert.Equal(t, nil, err)

	sfn := createTestStreamFunction("sfn-1", observedTag)
	err = sfn.Connect(ctx, testaddr)
	assert.NoError(t, err, "sfn connect must be success")

	// add a same name sfn to zipper.
	sameNameSfn := createTestStreamFunction("sfn-1", observedTag)
	sameNameSfn.SetDataFrameObserver(func(bf *frame.DataFrame) {
		assert.Equal(t, string(payload), string(bf.Payload))

		// panic test: reading array out of range.
		arr := []int{1, 2}
		t.Log(arr[100])
	})

	sameNameSfn.SetErrorHandler(func(err error) {
		if strings.HasPrefix(err.Error(), "yomo: stream panic") {
			assert.Regexp(
				t,
				`^yomo: stream panic: runtime error: index out of range \[100\] with length 2`,
				err.Error(),
			)
		}
	})

	err = sameNameSfn.Connect(ctx, testaddr)
	assert.NoError(t, err, "sfn connect should replace the old sfn stream")

	exited := checkClientExited(sfn, time.Second)
	assert.True(t, exited, "the old sfn stream should exited")

	exited = checkClientExited(sameNameSfn, time.Second)
	assert.False(t, exited, "the new sfn stream should not exited")

	mdBytes, _ := NewDefaultMetadata(source.clientID, "tid", "sid", false).Encode()

	err = sameNameSfn.WriteFrame(&frame.DataFrame{Tag: backflowTag, Metadata: mdBytes, Payload: backflow})
	assert.NoError(t, err)

	stats := server.StatsFunctions()
	nameList := []string{}
	for _, name := range stats {
		nameList = append(nameList, name)
	}
	assert.ElementsMatch(t, nameList, []string{"source", "sfn-1"})

	md := metadata.New(
		NewDefaultMetadata(source.clientID, "tid", "sid", false),
		metadata.M{
			"foo": "bar",
		},
	)
	mdBytes, _ = md.Encode()

	dataFrame := &frame.DataFrame{
		Tag:      observedTag,
		Metadata: mdBytes,
		Payload:  payload,
	}

	err = source.WriteFrame(dataFrame)
	assert.NoError(t, err, "source write dataFrame must be success")

	time.Sleep(2 * time.Second)

	reocrdTag, reocrdMD, reocrdPayload := recorder.dataFrameContent()
	assert.Equal(t, reocrdTag, dataFrame.Tag)
	// raw metadata cant be compared because metadata is not ordered.
	assert.Equal(t, reocrdMD, md)
	assert.Equal(t, reocrdPayload, dataFrame.Payload)

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

func createTestStreamFunction(name string, observedTag frame.Tag) *Client {
	sfn := NewClient(
		name,
		StreamTypeStreamFunction,
		WithCredential("token:auth-token"),
		WithLogger(discardingLogger),
	)
	sfn.SetObserveDataTags(observedTag)

	return sfn
}

// frameWriterRecorder frames be writen.
type frameWriterRecorder struct {
	name         string
	codec        frame.Codec
	packetReader frame.PacketReadWriter
	mu           sync.Mutex
	buf          *bytes.Buffer
}

func newFrameWriterRecorder(name string) *frameWriterRecorder {
	return &frameWriterRecorder{
		name:         name,
		codec:        y3codec.Codec(),
		packetReader: y3codec.PacketReadWriter(),
		buf:          new(bytes.Buffer),
	}
}

func (w *frameWriterRecorder) Name() string                              { return w.name }
func (w *frameWriterRecorder) Close() error                              { return nil }
func (w *frameWriterRecorder) Connect(_ context.Context, _ string) error { return nil }

func (w *frameWriterRecorder) WriteFrame(f frame.Frame) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	b, _ := w.codec.Encode(f)
	_, err := w.buf.Write(b)

	return err
}

func (w *frameWriterRecorder) dataFrameContent() (frame.Tag, metadata.M, []byte) {
	w.mu.Lock()
	defer w.mu.Unlock()

	dataFrame := new(frame.DataFrame)
	y3codec.Codec().Decode(w.buf.Bytes(), dataFrame)
	md, _ := metadata.Decode(dataFrame.Metadata)
	return dataFrame.Tag, md, dataFrame.Payload
}
