package core

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
)

func TestDataStream(t *testing.T) {
	var (
		readBytes = []byte("aaabbbcccdddeeefff")
		name      = "test-data-stream"
		id        = "123456"
		styp      = StreamTypeStreamFunction
		observed  = []uint32{1, 2, 3}
		md        metadata.Metadata
	)

	// Create a stream that initializes the read buffer with a string that has been split by spaces.
	mockStream := newMemByteStream(readBytes)

	// create frame stream.
	frameStream := NewFrameStream(mockStream, &byteCodec{}, &bytePacketReadWriter{})

	stream := newDataStream(name, id, styp, md, observed, frameStream)

	t.Run("StreamInfo", func(t *testing.T) {
		assert.Equal(t, id, stream.ID())
		assert.Equal(t, name, stream.Name())
		assert.Equal(t, styp, stream.StreamType())
		assert.Equal(t, md, stream.Metadata())
		assert.Equal(t, observed, stream.ObserveDataTags())
	})

	t.Run("data stream read", func(t *testing.T) {
		gots := []byte{}
		for i := 0; i < len(readBytes)+1; i++ {
			f, err := stream.ReadFrame()
			if err != nil {
				if i == len(readBytes) {
					assert.Equal(t, io.EOF, err)
				} else {
					t.Fatal(err)
				}
				return
			}

			b, err := frameStream.codec.Encode(f)
			assert.NoError(t, err)

			gots = append(gots, b...)
		}
		assert.Equal(t, readBytes, gots)
	})

	t.Run("data stream write", func(t *testing.T) {
		dataWrited := []byte("ggghhhiiigggkkklll")

		for _, w := range dataWrited {
			err := stream.WriteFrame(byteFrame(w))
			assert.NoError(t, err)
		}

		assert.Equal(t, string(mockStream.GetReadBytes()), string(dataWrited))
	})

	t.Run("data stream close", func(t *testing.T) {
		err := stream.Close()
		assert.NoError(t, err)

		// close twice.
		err = stream.Close()
		assert.NoError(t, err)

		f, err := stream.ReadFrame()
		assert.ErrorIs(t, err, io.EOF)
		assert.Nil(t, f)

		err = stream.WriteFrame(byteFrame('a'))
		assert.ErrorIs(t, err, io.EOF)

		select {
		case <-stream.Context().Done():
		default:
			assert.Fail(t, "stream.Context().Done() should be done")
		}
	})
}

func TestStreamTypeString(t *testing.T) {
	assert.Equal(t, StreamTypeSource.String(), "Source")
	assert.Equal(t, StreamTypeStreamFunction.String(), "StreamFunction")
	assert.Equal(t, StreamTypeUpstreamZipper.String(), "UpstreamZipper")
	assert.Equal(t, StreamType(0).String(), "Unknown")
}

// byteFrame implements frame.Frame interface for unittest.
func byteFrame(byt byte) *frame.DataFrame {
	return &frame.DataFrame{
		Meta:    new(frame.MetaFrame),
		Payload: &frame.PayloadFrame{Carriage: []byte{byt}},
	}
}

type byteCodec struct{}

var _ frame.Codec = &byteCodec{}

// Decode implements frame.Codec
func (*byteCodec) Decode(data []byte, f frame.Frame) error {
	df, ok := f.(*frame.DataFrame)
	if !ok {
		return nil
	}
	df.Payload.Carriage = data

	return nil
}

// Encode implements frame.Codec
func (*byteCodec) Encode(f frame.Frame) ([]byte, error) {
	return f.(*frame.DataFrame).Payload.Carriage, nil
}

type bytePacketReadWriter struct{}

// WritePacket implements frame.PacketReadWriter
func (*bytePacketReadWriter) WritePacket(stream io.Writer, ftyp frame.Type, data []byte) error {
	_, err := stream.Write(data)
	return err
}

// ReadPacket implements frame.PacketReadWriter
func (*bytePacketReadWriter) ReadPacket(stream io.Reader) (frame.Type, []byte, error) {
	var b [1]byte
	_, err := stream.Read(b[:])
	if err != nil {
		return frame.TypeDataFrame, nil, err
	}
	return frame.TypeDataFrame, []byte{b[0]}, nil
}

var _ frame.PacketReadWriter = &bytePacketReadWriter{}

type memByteStream struct {
	ctx      context.Context
	cancel   context.CancelFunc
	readBuf  *bytes.Buffer
	writeBuf *bytes.Buffer
	mutex    sync.Mutex
}

func newMemByteStream(readInitBytes []byte) *memByteStream {
	ctx, cancel := context.WithCancel(context.Background())
	return &memByteStream{
		ctx:      ctx,
		cancel:   cancel,
		readBuf:  bytes.NewBuffer(readInitBytes),
		writeBuf: &bytes.Buffer{},
	}
}

func (rw *memByteStream) Context() context.Context { return rw.ctx }

func (rw *memByteStream) Read(p []byte) (n int, err error) {
	select {
	case <-rw.ctx.Done():
		return 0, io.EOF
	default:
	}

	rw.mutex.Lock()
	defer rw.mutex.Unlock()
	return rw.readBuf.Read(p)
}

func (rw *memByteStream) Write(p []byte) (n int, err error) {
	select {
	case <-rw.ctx.Done():
		return 0, io.EOF
	default:
	}

	rw.mutex.Lock()
	defer rw.mutex.Unlock()
	return rw.writeBuf.Write(p)
}

func (rw *memByteStream) Close() error {
	rw.cancel()
	select {
	case <-rw.ctx.Done():
		return nil
	default:
	}

	rw.mutex.Lock()
	defer rw.mutex.Unlock()
	rw.writeBuf.Reset()
	return nil
}

func (rw *memByteStream) GetReadBytes() []byte {
	rw.mutex.Lock()
	defer rw.mutex.Unlock()
	return rw.writeBuf.Bytes()
}
