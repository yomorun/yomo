package core

import (
	"errors"
	"io"
	"sync"

	"github.com/yomorun/yomo/core/frame"
)

// FrameStream is the QUIC Stream with the minimum unit Frame.
type FrameStream struct {
	// Stream is a QUIC stream.
	stream io.ReadWriter
	mu     sync.Mutex
}

// NewFrameStream creates a new FrameStream.
func NewFrameStream(s io.ReadWriter) *FrameStream {
	return &FrameStream{
		stream: s,
	}
}

// ReadFrame reads next frame from QUIC stream.
func (fs *FrameStream) ReadFrame() (frame.Frame, error) {
	if fs.stream == nil {
		return nil, errors.New("core.ReadStream: stream can not be nil")
	}
	return ParseFrame(fs.stream)
}

// WriteFrame writes a frame into QUIC stream.
func (fs *FrameStream) WriteFrame(f frame.Frame) (int, error) {
	if fs.stream == nil {
		return 0, errors.New("core.WriteFrame: stream can not be nil")
	}
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.stream.Write(f.Encode())
}
