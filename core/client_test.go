package core

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/lucas-clemente/quic-go"
	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/core/router"
	"github.com/yomorun/yomo/pkg/config"
	"github.com/yomorun/yomo/pkg/logger"
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
		WithLogger(logger.Default()),
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

// mockFrameReader mock a FrameReader
type mockFrameReader struct {
	mu        sync.Mutex
	seq       int
	intervals time.Duration
	frames    []frame.Frame
}

func newMockFrameReader(intervals time.Duration, frames ...frame.Frame) *mockFrameReader {
	return &mockFrameReader{
		intervals: intervals,
		frames:    frames,
	}
}

func (r *mockFrameReader) ReadFrame() (frame.Frame, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.seq >= len(r.frames) {
		return nil, errors.New("all data has been read")
	}

	time.Sleep(r.intervals)

	frm := r.frames[r.seq]
	r.seq++

	return frm, nil
}

func TestClientWaitHandshakeAck(t *testing.T) {
	type fields struct {
		frames    []frame.Frame
		intervals time.Duration
	}
	type args struct {
		timeout time.Duration
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr error
	}{
		{
			name: "handshake ack timeout",
			fields: fields{
				frames:    []frame.Frame{frame.NewGoawayFrame("goaway"), frame.NewHandshakeAckFrame()},
				intervals: time.Second,
			},
			args: args{
				timeout: time.Millisecond,
			},
			wantErr: ErrHandshakeAckTimeout,
		},
		{
			name: "handshake ack success",
			fields: fields{
				frames:    []frame.Frame{frame.NewGoawayFrame("goaway"), frame.NewHandshakeAckFrame()},
				intervals: time.Microsecond,
			},
			args: args{
				timeout: time.Millisecond,
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				client      = &Client{logger: logger.Default()}
				frameReader = newMockFrameReader(tt.fields.intervals, tt.fields.frames...)
			)

			err := client.waitHandshakeAck(frameReader, tt.args.timeout)
			assert.Equal(t, tt.wantErr, err)
		})
	}
}
