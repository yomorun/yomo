package core

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
)

func TestConnection(t *testing.T) {
	var (
		readBytes = []byte("aaabbbcccdddeeefff")
		name      = "test-data-connection"
		id        = "123456"
		styp      = ClientTypeStreamFunction
		observed  = []uint32{1, 2, 3}
		md        metadata.M
	)

	// Create a connection that initializes the read buffer with a string that has been split by spaces.
	mockStream := newMemByteStream(readBytes)

	// create frame connection.
	fs := NewFrameStream(mockStream, &byteCodec{}, &bytePacketReadWriter{})

	connection := newConnection(name, id, styp, md, observed, nil, fs)

	t.Run("ConnectionInfo", func(t *testing.T) {
		assert.Equal(t, id, connection.ID())
		assert.Equal(t, name, connection.Name())
		assert.Equal(t, styp, connection.ClientType())
		assert.Equal(t, md, connection.Metadata())
		assert.Equal(t, observed, connection.ObserveDataTags())
	})

	t.Run("connection read", func(t *testing.T) {
		gots := []byte{}
		for i := 0; i < len(readBytes)+1; i++ {
			f, err := connection.ReadFrame()
			if err != nil {
				if i == len(readBytes) {
					assert.Equal(t, io.EOF, err)
				} else {
					t.Fatal(err)
				}
				return
			}

			b, err := fs.codec.Encode(f)
			assert.NoError(t, err)

			gots = append(gots, b...)
		}
		assert.Equal(t, readBytes, gots)
	})

	t.Run("connection write", func(t *testing.T) {
		dataWrited := []byte("ggghhhiiigggkkklll")

		for _, w := range dataWrited {
			err := connection.WriteFrame(byteFrame(w))
			assert.NoError(t, err)
		}

		assert.Equal(t, string(mockStream.GetReadBytes()), string(dataWrited))
	})

	t.Run("connection close", func(t *testing.T) {
		err := connection.Close()
		assert.NoError(t, err)

		// close twice.
		err = connection.Close()
		assert.NoError(t, err)

		f, err := connection.ReadFrame()
		assert.ErrorIs(t, err, io.EOF)
		assert.Nil(t, f)

		err = connection.WriteFrame(byteFrame('a'))
		assert.ErrorIs(t, err, io.EOF)

		select {
		case <-connection.Context().Done():
		default:
			assert.Fail(t, "stream.Context().Done() should be done")
		}
	})
}

func TestClientTypeString(t *testing.T) {
	assert.Equal(t, ClientTypeSource.String(), "Source")
	assert.Equal(t, ClientTypeStreamFunction.String(), "StreamFunction")
	assert.Equal(t, ClientTypeUpstreamZipper.String(), "UpstreamZipper")
	assert.Equal(t, ClientType(0).String(), "Unknown")
}

// byteFrame implements frame.Frame interface for unittest.
func byteFrame(byt byte) *frame.DataFrame {
	return &frame.DataFrame{
		Payload: []byte{byt},
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
	df.Payload = data

	return nil
}

// Encode implements frame.Codec
func (*byteCodec) Encode(f frame.Frame) ([]byte, error) {
	return f.(*frame.DataFrame).Payload, nil
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

// CancelRead implements quic.Stream.
func (*memByteStream) CancelRead(quic.StreamErrorCode) {
	panic("unimplemented")
}

func (*memByteStream) CancelWrite(quic.StreamErrorCode) {
	panic("unimplemented")
}

func (*memByteStream) SetDeadline(t time.Time) error {
	panic("unimplemented")
}

func (*memByteStream) SetReadDeadline(t time.Time) error {
	panic("unimplemented")
}

func (*memByteStream) SetWriteDeadline(t time.Time) error {
	panic("unimplemented")
}

func (*memByteStream) StreamID() quic.StreamID {
	panic("unimplemented")
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
