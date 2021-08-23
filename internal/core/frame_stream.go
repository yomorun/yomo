package core

import (
	"errors"
	"io"

	"github.com/yomorun/yomo/internal/frame"
)

// FrameStream is the QUIC Stream with the minimum unit Frame.
type FrameStream struct {
	// Stream is a QUIC stream.
	stream io.ReadWriter
}

// NewFrameStream creates a new FrameStream.
func NewFrameStream(s io.ReadWriter) *FrameStream {
	return &FrameStream{
		stream: s,
	}
}

// Read next frame from QUIC stream.
func (fs *FrameStream) ReadFrame() (frame.Frame, error) {
	if fs.stream == nil {
		return nil, errors.New("stream can not be nil")
	}
	return ParseFrame(fs.stream)
}

// Write a frame into QUIC stream.
func (fs *FrameStream) WriteFrame(f frame.Frame) (int, error) {
	if fs.stream == nil {
		return 0, errors.New("stream can not be nil")
	}
	return fs.stream.Write(f.Encode())
}

// // Close the frame stream.
// func (fs *FrameStream) Close() error {
// 	if fs.stream == nil {
// 		return nil
// 	}
// 	return fs.stream.Close()
// }
