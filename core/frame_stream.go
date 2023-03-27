package core

import (
	"errors"
	"io"
	"sync"

	"github.com/yomorun/yomo/core/frame"
)

// FrameStream is the frame.ReadWriter that goroutinue read write safely.
type FrameStream struct {
	stream io.ReadWriter
	mu     sync.Mutex
}

// NewFrameStream creates a new FrameStream.
func NewFrameStream(s io.ReadWriter) frame.ReadWriter {
	return &FrameStream{stream: s}
}

// ReadFrame reads next frame from underlying stream.
func (fs *FrameStream) ReadFrame() (frame.Frame, error) {
	if fs.stream == nil {
		return nil, errors.New("core.ReadStream: stream can not be nil")
	}
	return ParseFrame(fs.stream)
}

// WriteFrame writes a frame into underlying stream.
func (fs *FrameStream) WriteFrame(f frame.Frame) error {
	if fs.stream == nil {
		return errors.New("core.WriteFrame: stream can not be nil")
	}
	fs.mu.Lock()
	defer fs.mu.Unlock()

	_, err := fs.stream.Write(f.Encode())
	return err
}
