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
func NewFrameStream(s io.ReadWriter) FrameReadwriter {
	return &FrameStream{
		stream: s,
		mu:     sync.Mutex{},
	}
}

// FrameReadwriter is the interface that groups the ReadFrame and WriteFrame methods.
type FrameReadwriter interface {
	FrameReader
	FrameWriter
}

// FrameReader reads frame from underlying stream.
type FrameReader interface {
	// ReadFrame reads frame, if error, the error returned is not empty
	// and frame returned is nil.
	ReadFrame() (frame.Frame, error)
}

// ReadFrame reads next frame from QUIC stream.
func (fs *FrameStream) ReadFrame() (frame.Frame, error) {
	if fs.stream == nil {
		return nil, errors.New("core.ReadStream: stream can not be nil")
	}
	return ParseFrame(fs.stream)
}

// FrameWriter writes frame to underlying stream.
type FrameWriter interface {
	// WriteFrame writes frame, It returns frame byte size and a write error.
	WriteFrame(f frame.Frame) (int, error)
}

// WriteFrame writes a frame into underlying stream.
func (fs *FrameStream) WriteFrame(f frame.Frame) (int, error) {
	if fs.stream == nil {
		return 0, errors.New("core.WriteFrame: stream can not be nil")
	}
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.stream.Write(f.Encode())
}
