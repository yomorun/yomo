package core

import (
	"context"
	"io"
	"sync"

	"github.com/yomorun/yomo/core/frame"
)

// FrameStream is the frame.ReadWriter that goroutinue read write safely.
type FrameStream struct {
	codec        frame.Codec
	packetReader frame.PacketReader

	// mu protected stream write and close
	// because of stream write and close is not goroutinue-safely.
	mu         sync.Mutex
	underlying ContextReadWriteCloser
}

// NewFrameStream creates a new FrameStream.
func NewFrameStream(
	stream ContextReadWriteCloser, codec frame.Codec, packetReader frame.PacketReader,
) *FrameStream {
	return &FrameStream{
		underlying:   stream,
		codec:        codec,
		packetReader: packetReader,
	}
}

func (fs *FrameStream) Context() context.Context {
	return fs.underlying.Context()
}

// ReadFrame reads next frame from underlying stream.
func (fs *FrameStream) ReadFrame() (frame.Frame, error) {
	select {
	case <-fs.underlying.Context().Done():
		return nil, io.EOF
	default:
	}

	fType, b, err := fs.packetReader.ReadPacket(fs.underlying)
	if err != nil {
		return nil, err
	}

	f, err := frame.NewFrame(fType)
	if err != nil {
		return nil, err
	}

	if err := fs.codec.Decode(b, f); err != nil {
		return nil, err
	}

	return f, nil
}

// WriteFrame writes a frame into underlying stream.
func (fs *FrameStream) WriteFrame(f frame.Frame) error {
	select {
	case <-fs.underlying.Context().Done():
		return io.EOF
	default:
	}

	fs.mu.Lock()
	defer fs.mu.Unlock()

	b, err := fs.codec.Encode(f)
	if err != nil {
		return err
	}

	_, err = fs.underlying.Write(b)
	return err
}

func (fs *FrameStream) Close() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	return fs.underlying.Close()
}
