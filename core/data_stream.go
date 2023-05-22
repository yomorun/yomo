package core

import (
	"context"
	"io"

	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
)

// StreamInfo holds the information of DataStream.
type StreamInfo interface {
	// Name returns the name of the stream, which is set by clients.
	Name() string
	// ID represents the dataStream ID, the ID is an unique string.
	ID() string
	// StreamType represents dataStream type (Source | SFN | UpstreamZipper).
	StreamType() StreamType
	// Metadata returns the extra info of the application.
	// The metadata is a merged set of data from both the handshake and authentication processes.
	Metadata() metadata.Metadata
	// ObserveDataTags observed data tags.
	// TODO: There maybe a sorted list, we can find tag quickly.
	ObserveDataTags() []frame.Tag
}

// DataStream wraps the specific io stream (typically quic.Stream) to transfer frames.
// DataStream be used to read and write frames, and be managed by Connector.
type DataStream interface {
	Context() context.Context
	StreamInfo
	frame.ReadWriteCloser
}

type dataStream struct {
	name       string
	id         string
	streamType StreamType
	metadata   metadata.Metadata
	observed   []frame.Tag

	stream *FrameStream
}

// newDataStream constructures dataStream.
func newDataStream(
	name string,
	id string,
	streamType StreamType,
	metadata metadata.Metadata,
	observed []frame.Tag,
	stream *FrameStream,
) DataStream {
	return &dataStream{
		name:       name,
		id:         id,
		streamType: streamType,
		metadata:   metadata,
		observed:   observed,
		stream:     stream,
	}
}

// DataStream implements.
func (s *dataStream) Context() context.Context        { return s.stream.Context() }
func (s *dataStream) ID() string                      { return s.id }
func (s *dataStream) Name() string                    { return s.name }
func (s *dataStream) Metadata() metadata.Metadata     { return s.metadata }
func (s *dataStream) StreamType() StreamType          { return s.streamType }
func (s *dataStream) ObserveDataTags() []frame.Tag    { return s.observed }
func (s *dataStream) WriteFrame(f frame.Frame) error  { return s.stream.WriteFrame(f) }
func (s *dataStream) ReadFrame() (frame.Frame, error) { return s.stream.ReadFrame() }
func (s *dataStream) Close() error                    { return s.stream.Close() }

const (
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

var streamTypeStringMap = map[StreamType]string{
	StreamTypeSource:         "Source",
	StreamTypeUpstreamZipper: "UpstreamZipper",
	StreamTypeStreamFunction: "StreamFunction",
}

// String returns string for StreamType.
func (c StreamType) String() string {
	str, ok := streamTypeStringMap[c]
	if !ok {
		return "Unknown"
	}
	return str
}

// ContextReadWriteCloser represents a stream which its lifecycle managed by context.
// The context should be closed when the stream is closed.
type ContextReadWriteCloser interface {
	// Context returns the context which manage the lifecycle of stream.
	Context() context.Context
	// The stream.
	io.ReadWriteCloser
}
