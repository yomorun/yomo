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

const (
	testaddr     = "127.0.0.1:19999"
	redirectAddr = "127.0.0.1:19998"
)

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

func TestConnectTo(t *testing.T) {
	t.Parallel()
	connectToEndpoint := "127.0.0.1:19996"
	go func() {
		srv := NewServer("zipper", WithServerLogger(discardingLogger))
		srv.ConfigVersionNegotiateFunc(func(cVersion, sVersion string) error {
			return &ErrConnectTo{connectToEndpoint}
		})
		srv.ListenAndServe(context.TODO(), redirectAddr)
	}()

	source := NewClient(
		"source",
		redirectAddr,
		ClientTypeSource,
		WithLogger(discardingLogger),
	)

	_ = source.Connect(context.TODO())

	assert.Equal(t, source.zipperAddr, connectToEndpoint)
}

func TestFrameRoundTrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	var (
		sourceToSfn1Tag     = frame.Tag(0x13)
		Sfn1ToSfn2Tag       = frame.Tag(0x14)
		sourceToSfn1Payload = []byte("source -> sfn1")
		Sfn1ToSfn2Payload   = []byte("sfn1 -> sfn2")
	)

	// test server hooks
	ht := &hookTester{t: t}

	server := NewServer("zipper",
		WithAuth("token", "auth-token"),
		WithServerQuicConfig(DefaultQuicConfig),
		WithServerTLSConfig(nil),
		WithServerLogger(discardingLogger),
		WithConnMiddleware(ht.connMiddleware),
		WithFrameMiddleware(ht.frameMiddleware),
	)
	server.ConfigRouter(router.Default())
	server.ConfigVersionNegotiateFunc(DefaultVersionNegotiateFunc)

	recorder := newFrameWriterRecorder("mockID", "mockClientLocal", "mockClientRemote")
	server.AddDownstreamServer(recorder)

	assert.Equal(t, server.Downstreams()["mockClientLocal"], recorder.ID())

	go func() {
		err := server.ListenAndServe(ctx, testaddr)
		fmt.Println(err)
	}()

	illegalTokenSource := NewClient("source", testaddr, ClientTypeSource, WithCredential("token:error-token"), WithLogger(discardingLogger))
	err := illegalTokenSource.Connect(ctx)
	assert.Equal(t, "authentication failed: client credential type is token", err.Error())

	source := NewClient(
		"source",
		testaddr,
		ClientTypeSource,
		WithCredential("token:auth-token"),
		WithClientQuicConfig(DefaultClientQuicConfig),
		WithClientTLSConfig(nil),
		WithLogger(discardingLogger),
		WithReConnect(),
		WithNonBlockWrite(),
	)

	err = source.Connect(ctx)
	assert.NoError(t, err, "source connect must be success")
	closeEarlySfn := createTestStreamFunction("close-early-sfn", testaddr, 0x15)
	closeEarlySfn.Connect(ctx)
	assert.Equal(t, nil, err)

	// test close early.
	closeEarlySfn.Close()
	assert.Equal(t, nil, err)

	exited := checkClientExited(closeEarlySfn, time.Second)
	assert.True(t, exited, "close-early-sfn should exited")

	// sfn to zipper.
	sfn1 := createTestStreamFunction("sfn-1", testaddr, sourceToSfn1Tag)
	sfn1.SetDataFrameObserver(func(bf *frame.DataFrame) {
		assert.Equal(t, sourceToSfn1Tag, bf.Tag)
		assert.Equal(t, string(sourceToSfn1Payload), string(bf.Payload))

		// panic test: reading array out of range.
		arr := []int{1, 2}
		t.Log(arr[100])
	})

	sfn1.SetErrorHandler(func(err error) {
		if strings.HasPrefix(err.Error(), "yomo: stream panic") {
			assert.Regexp(
				t,
				`^yomo: stream panic: runtime error: index out of range \[100\] with length 2`,
				err.Error(),
			)
		}
	})
	err = sfn1.Connect(ctx)
	assert.NoError(t, err, "sfn-1 connect should succeed")

	exited = checkClientExited(sfn1, time.Second)
	assert.False(t, exited, "sfn stream should not exited")

	sfn2 := createTestStreamFunction("sfn-2", testaddr, Sfn1ToSfn2Tag)
	sfn2.SetDataFrameObserver(func(bf *frame.DataFrame) {
		assert.Equal(t, Sfn1ToSfn2Tag, bf.Tag)
		assert.Equal(t, string(Sfn1ToSfn2Payload), string(bf.Payload))
	})

	err = sfn2.Connect(ctx)
	assert.NoError(t, err, "sfn-2 connect should succeed")

	exited = checkClientExited(sfn2, time.Second)
	assert.False(t, exited, "sfn stream should not exited")

	sfnMd := NewMetadata(source.clientID, "tid")

	sfnMetaBytes, _ := sfnMd.Encode()

	dataFrame := &frame.DataFrame{
		Tag:      sourceToSfn1Tag,
		Metadata: sfnMetaBytes,
		Payload:  sourceToSfn1Payload,
	}

	err = sfn1.WriteFrame(dataFrame)
	assert.NoError(t, err)

	assertDownstreamDataFrame(t, dataFrame.Tag, sfnMd, dataFrame.Payload, recorder)

	stats := server.StatsFunctions()
	nameList := []string{}
	for _, name := range stats {
		nameList = append(nameList, name)
	}
	assert.ElementsMatch(t, nameList, []string{"source", "sfn-1", "sfn-2"})

	md := metadata.New(
		NewMetadata(source.clientID, "tid"),
		metadata.M{
			"foo": "bar",
		},
	)
	sourceMetaBytes, _ := md.Encode()

	dataFrame = &frame.DataFrame{
		Tag:      Sfn1ToSfn2Tag,
		Metadata: sourceMetaBytes,
		Payload:  Sfn1ToSfn2Payload,
	}

	err = source.WriteFrame(dataFrame)
	assert.NoError(t, err, "source write dataFrame must be success")

	assertDownstreamDataFrame(t, dataFrame.Tag, md, dataFrame.Payload, recorder)

	assert.NoError(t, source.Close(), "source client.Close() should not return error")
	assert.NoError(t, sfn1.Close(), "sfn-1 client.Close() should not return error")
	assert.NoError(t, sfn2.Close(), "sfn-2 client.Close() should not return error")
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
	mu        sync.Mutex
	connNames []string
	t         *testing.T
}

func (a *hookTester) connMiddleware(next ConnHandler) ConnHandler {
	return func(c *Connection) {
		a.mu.Lock()
		if a.connNames == nil {
			a.connNames = make([]string, 0)
		}
		a.connNames = append(a.connNames, c.Name())
		a.mu.Unlock()

		next(c)

		a.mu.Lock()
		assert.Contains(a.t, a.connNames, c.Name())
		a.mu.Unlock()
	}
}

func (a *hookTester) frameMiddleware(next FrameHandler) FrameHandler {
	return func(c *Context) {
		c.Set("a", "b")
		next(c)
		v, ok := c.Get("a")
		assert.True(a.t, ok)
		assert.Equal(a.t, "b", v)
	}
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

// frameWriterRecorder frames be written.
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
