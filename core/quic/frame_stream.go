package quic

import (
	"errors"

	"github.com/yomorun/yomo/core/parser"
	"github.com/yomorun/yomo/internal/frame"
)

// FrameStream is the QUIC Stream with the minimum unit Frame.
type FrameStream struct {
	// Stream is a QUIC stream.
	stream Stream
}

// NewFrameStream creates a new FrameStream.
func NewFrameStream(stream Stream) *FrameStream {
	return &FrameStream{
		stream: stream,
	}
}

// Read next frame from QUIC stream.
func (fs *FrameStream) Read() (frame.Frame, error) {
	if fs.stream == nil {
		return nil, errors.New("Stream is nil")
	}
	return parser.ParseFrame(fs.stream)
}

// Write a frame into QUIC stream.
func (fs *FrameStream) Write(f frame.Frame) (int, error) {
	if fs.stream == nil {
		return 0, errors.New("Stream is nil")
	}
	return fs.stream.Write(f.Encode())
}

// Close the frame stream.
func (fs *FrameStream) Close() error {
	if fs.stream == nil {
		return nil
	}
	return fs.stream.Close()
}
