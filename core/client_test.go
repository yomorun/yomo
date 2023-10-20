package core

import (
	"bytes"
	"context"
	"fmt"
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
	"github.com/yomorun/yomo/pkg/frame-codec/y3codec"
)

const testaddr = "127.0.0.1:19999"

var discardingLogger = ylog.NewFromConfig(ylog.Config{Output: "/dev/null", ErrorOutput: "/dev/null"})

// debugLogger be used to debug unittest.
// var debugLogger = ylog.NewFromConfig(ylog.Config{Verbose: true, Level: "debug"})

func TestClientDialNothing(t *testing.T) {
	ctx := context.Background()

	client := NewClient("source", testaddr, ClientTypeSource, WithLogger(discardingLogger))
	err := client.Connect(ctx)

	qerr := net.ErrClosed
	assert.ErrorAs(t, err, &qerr, "dial must timeout")
}

func TestFrameRoundTrip(t *testing.T) {
	ctx := context.Background()

	var (
		observedTag = frame.Tag(0x13)
		backflowTag = frame.Tag(0x14)
		payload     = []byte("hello data frame")
		backflow    = []byte("hello backflow frame")
	)

	server := NewServer("zipper",
		WithAuth("token", "auth-token"),
		WithServerQuicConfig(DefalutQuicConfig),
		WithServerTLSConfig(nil),
		WithServerLogger(discardingLogger),
	)
	server.ConfigRouter(router.Default())

	// test server hooks
	ht := &hookTester{t}
	server.SetStartHandlers(ht.startHandler)
	server.SetBeforeHandlers(ht.beforeHandler)
	server.SetAfterHandlers(ht.afterHandler)

	recorder := newFrameWriterRecorder("mockID", "mockClientLocal", "mockClientRemote")
	server.AddDownstreamServer(recorder)

	assert.Equal(t, server.Downstreams()["mockID"], recorder.ID())

	go func() {
		err := server.ListenAndServe(ctx, testaddr)
		fmt.Println(err)
	}()

	illegalTokenSource := NewClient("source", testaddr, ClientTypeSource, WithCredential("token:error-token"), WithLogger(discardingLogger))
	err := illegalTokenSource.Connect(ctx)
	assert.Equal(t, "authentication failed: client credential name is token", err.Error())

	source := NewClient(
		"source",
		testaddr,
		ClientTypeSource,
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

	err = source.Connect(ctx)
	assert.NoError(t, err, "source connect must be success")
	closeEarlySfn := createTestStreamFunction("close-early-sfn", testaddr, observedTag)
	closeEarlySfn.Connect(ctx)
	assert.Equal(t, nil, err)

	// test close early.
	closeEarlySfn.Close()
	assert.Equal(t, nil, err)

	exited := checkClientExited(closeEarlySfn, time.Second)
	assert.True(t, exited, "close-early-sfn should exited")

	// sfn to zipper.
	sfn := createTestStreamFunction("sfn-1", testaddr, observedTag)
	sfn.SetDataFrameObserver(func(bf *frame.DataFrame) {
		assert.Equal(t, string(payload), string(bf.Payload))

		// panic test: reading array out of range.
		arr := []int{1, 2}
		t.Log(arr[100])
	})

	sfn.SetErrorHandler(func(err error) {
		if strings.HasPrefix(err.Error(), "yomo: stream panic") {
			assert.Regexp(
				t,
				`^yomo: stream panic: runtime error: index out of range \[100\] with length 2`,
				err.Error(),
			)
		}
	})

	err = sfn.Connect(ctx)
	assert.NoError(t, err, "sfn connect should replace the old sfn stream")

	exited = checkClientExited(sfn, time.Second)
	assert.False(t, exited, "sfn stream should not exited")

	sfnMd := NewDefaultMetadata(source.clientID, "sfn-tid", "sfn-sid", false)

	sfnMetaBytes, _ := sfnMd.Encode()

	dataFrame := &frame.DataFrame{
		Tag:      backflowTag,
		Metadata: sfnMetaBytes,
		Payload:  backflow,
	}

	err = sfn.WriteFrame(dataFrame)
	assert.NoError(t, err)

	assertDownstreamDataFrame(t, dataFrame.Tag, sfnMd, dataFrame.Payload, recorder)

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
	sourceMetaBytes, _ := md.Encode()

	dataFrame = &frame.DataFrame{
		Tag:      observedTag,
		Metadata: sourceMetaBytes,
		Payload:  payload,
	}

	err = source.WriteFrame(dataFrame)
	assert.NoError(t, err, "source write dataFrame must be success")

	assertDownstreamDataFrame(t, dataFrame.Tag, md, dataFrame.Payload, recorder)

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

func createTestStreamFunction(name string, zipperAddr string, observedTag frame.Tag) *Client {
	sfn := NewClient(
		name,
		zipperAddr,
		ClientTypeStreamFunction,
		WithCredential("token:auth-token"),
		WithLogger(discardingLogger),
	)
	sfn.SetObserveDataTags(observedTag)

	return sfn
}

// frameWriterRecorder frames be writen.
type frameWriterRecorder struct {
	id           string
	localName    string
	remoteName   string
	codec        frame.Codec
	packetReader frame.PacketReadWriter
	mu           sync.Mutex
	buf          *bytes.Buffer
}

func newFrameWriterRecorder(id, localName, remoteName string) *frameWriterRecorder {
	return &frameWriterRecorder{
		id:           id,
		localName:    localName,
		remoteName:   remoteName,
		codec:        y3codec.Codec(),
		packetReader: y3codec.PacketReadWriter(),
		buf:          new(bytes.Buffer),
	}
}

func (w *frameWriterRecorder) ID() string                      { return w.id }
func (w *frameWriterRecorder) LocalName() string               { return w.localName }
func (w *frameWriterRecorder) RemoteName() string              { return w.remoteName }
func (w *frameWriterRecorder) Close() error                    { return nil }
func (w *frameWriterRecorder) Connect(_ context.Context) error { return nil }

func (w *frameWriterRecorder) WriteFrame(f frame.Frame) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	b, _ := w.codec.Encode(f)
	err := w.packetReader.WritePacket(w.buf, f.Type(), b)

	return err
}

func (w *frameWriterRecorder) ReadFrameContent() (frame.Tag, metadata.M, []byte) {
	w.mu.Lock()
	defer w.mu.Unlock()

	dataFrame := new(frame.DataFrame)

	_, bytes, _ := w.packetReader.ReadPacket(w.buf)
	w.codec.Decode(bytes, dataFrame)
	md, _ := metadata.Decode(dataFrame.Metadata)
	return dataFrame.Tag, md, dataFrame.Payload
}

func assertDownstreamDataFrame(t *testing.T, tag uint32, md metadata.M, payload []byte, recorder *frameWriterRecorder) {
	// wait for the downstream to finish writing.
	time.Sleep(time.Second)

	recordTag, recordMD, recordPayload := recorder.ReadFrameContent()
	assert.Equal(t, recordTag, tag)
	assert.Equal(t, recordMD, md)
	assert.Equal(t, recordPayload, payload)
}
