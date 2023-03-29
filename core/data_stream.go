package core

import (
	"context"
	"io"
	"sync"
	"sync/atomic"

	"github.com/quic-go/quic-go"
	"github.com/yomorun/yomo/core/frame"
)

// DataStream wraps the specific io streams (typically quic.Stream) to transfer frames.
// DataStream be used to read and write frames, and be managed by Connector.
type DataStream interface {
	// Context returns context.Context to manages DataStream lifecycle.
	Context() context.Context
	// Name returns the name of the stream, which is set by clients.
	Name() string
	// ID represents the dataStream ID, the ID is an unique string.
	ID() string
	// StreamType represents dataStream type (Source | SFN | UpstreamZipper).
	StreamType() StreamType
	// Metadata returns the extra info of the application
	Metadata() []byte
	// Close real close DataStream,
	// The controlStream calls this function, If you want close a dataStream, to use
	// the CloseWithError api.
	io.Closer
	// CloseWithError close DataStream with an error string,
	// This function do not real close the underlying stream, It notices controlStream to
	// close itself, The controlStream must close underlying stream after receive CloseStreamFrame.
	CloseWithError(string) error
	// ReadWriter writes or reads frame to underlying stream.
	// Writing and Reading are both goroutine-safely handle frames to peer side.
	// ReadWriter returns stream closed error if stream is closed.
	frame.ReadWriter
	// ObserveDataTags observed data tags.
	// TODO: There maybe a sorted list, we can find tag quickly.
	ObserveDataTags() []frame.Tag
}

// TODO: dataStream sync.Pool wrap.
type dataStream struct {
	name       string
	id         string
	streamType StreamType
	metadata   []byte
	observed   []frame.Tag

	closed atomic.Bool
	// mu protected stream write and close
	// because of quic stream write and close is not goroutinue-safely.
	mu            sync.Mutex
	stream        quic.Stream
	controlStream ControlStream
}

// newDataStream constructures dataStream.
func newDataStream(
	name string,
	id string,
	streamType StreamType,
	metadata []byte,
	stream quic.Stream,
	observed []frame.Tag,
	controlStream ControlStream,
) DataStream {
	return &dataStream{
		name:          name,
		id:            id,
		streamType:    streamType,
		metadata:      metadata,
		stream:        stream,
		observed:      observed,
		controlStream: controlStream,
	}
}

// DataStream implements.
func (s *dataStream) Context() context.Context     { return s.stream.Context() }
func (s *dataStream) ID() string                   { return s.id }
func (s *dataStream) Name() string                 { return s.name }
func (s *dataStream) Metadata() []byte             { return s.metadata }
func (s *dataStream) StreamType() StreamType       { return s.streamType }
func (s *dataStream) ObserveDataTags() []frame.Tag { return s.observed }

func (s *dataStream) WriteFrame(frm frame.Frame) error {
	if s.closed.Load() {
		return io.EOF
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.stream.Write(frm.Encode())
	return err
}

func (s *dataStream) ReadFrame() (frame.Frame, error) {
	if s.closed.Load() {
		return nil, io.EOF
	}
	return ParseFrame(s.stream)
}

func (s *dataStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Close the stream truly,
	// This function should be called after controlStream receive a closeStreamFrame.
	return s.stream.Close()
}

func (s *dataStream) CloseWithError(errString string) error {
	if s.closed.Load() {
		return nil
	}
	s.closed.Store(true)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Only notice client-side controlStream the stream has been closed.
	// The controlStream reads closeStreamFrame and to close dataStream.
	return s.controlStream.CloseStream(s.id, errString)
}

const (
	// StreamTypeNone is stream type "None".
	// "None" stream is not supposed to be in the yomo system.
	StreamTypeNone StreamType = 0xFF

	// StreamTypeSource is stream type "Source".
	// "Source" type stream sends data to "Stream Function" stream generally.
	StreamTypeSource StreamType = 0x5F

	// StreamTypeUpstreamZipper is connection type "Upstream Zipper".
	// "Upstream Zipper" type stream sends data from "Source" to other zipper node.
	// With "Upstream Zipper", the yomo can run in mesh mode.
	StreamTypeUpstreamZipper StreamType = 0x5E

	// StreamTypeStreamFunction is stream type "Stream Function".
	// "Stream Function" handles data from source.
	StreamTypeStreamFunction StreamType = 0x5D
)

// StreamType represents the stream type.
type StreamType byte

// String returns string for StreamType.
func (c StreamType) String() string {
	switch c {
	case StreamTypeSource:
		return "Source"
	case StreamTypeUpstreamZipper:
		return "Upstream Zipper"
	case StreamTypeStreamFunction:
		return "Stream Function"
	default:
		return "None"
	}
}
