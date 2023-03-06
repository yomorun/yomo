package core

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/quic-go/quic-go"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
	"golang.org/x/exp/slog"
)

// ErrStreamClosed be returned if dataStream has be closed.
var ErrStreamClosed = errors.New("yomo: dataStream closed")

// StreamInfo holds DataStream informations.
type StreamInfo interface {
	// Name returns the name of the stream, which is set by clients.
	Name() string
	// ID represents the dataStream ID, the ID is an unique string.
	ID() string
	// StreamType represents dataStream type (Source | SFN | UpstreamZipper).
	StreamType() StreamType
	// Metadata returns the extra info of the application
	Metadata() metadata.Metadata
}

// DataStream wraps the specific io streams (typically quic.Stream) to transfer y3 frames.
type DataStream interface {
	// Context returns context.Context to manages DataStream lifecycle.
	Context() context.Context
	// Close close DataStream,
	// reading or writing stream returns stream close error if stream is closed,.
	io.Closer
	StreamInfo() StreamInfo
	// ReadWriter writes or reads frame to underlying stream.
	// Writing and Reading are both goroutine-safely handle frames to peer side.
	frame.ReadWriter
	// ObserveDataTags observed data tags.
	// TODO: There maybe a sorted list, we can find tag quickly.
	ObserveDataTags() []frame.Tag
}

type dataStream struct {
	info     *streamInfo
	stream   quic.Stream
	observed []frame.Tag // observed data tags

	// mu protects closed and the read and write of the stream .
	mu     sync.Mutex
	closed bool

	logger *slog.Logger
}

// Close close DataStream, Reading and Writing to
// stream will return ErrStreamClosed if stream has be closed.
func (s *dataStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.closed {
		return nil
	}
	s.closed = true
	return s.stream.Close()
}

// newDataStream constructures dataStream.
func newDataStream(
	name string,
	id string,
	streamType StreamType,
	metadata metadata.Metadata,
	stream quic.Stream,
	observed []frame.Tag,
	logger *slog.Logger,
) DataStream {
	logger.Debug("new data stream")
	return &dataStream{
		info: &streamInfo{
			name:       name,
			id:         id,
			streamType: streamType,
			metadata:   metadata,
		},
		stream:   stream,
		observed: observed,
		logger:   logger,
	}
}

func (s *dataStream) StreamInfo() StreamInfo       { return s.info }
func (s *dataStream) Context() context.Context     { return s.stream.Context() }
func (s *dataStream) ObserveDataTags() []frame.Tag { return s.observed }

func (s *dataStream) WriteFrame(frm frame.Frame) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStreamClosed
	}
	_, err := s.stream.Write(frm.Encode())
	return err
}

func (s *dataStream) ReadFrame() (frame.Frame, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, ErrStreamClosed
	}
	return ParseFrame(s.stream)
}

const (
	// StreamTypeNone is stream type "None".
	// "None" stream is not supposed to be in the yomo system.
	StreamTypeNone StreamType = 0xFF

	// ClientTypeSource is stream type "Source".
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

// ClientType represents the stream type.
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

// streamInfo stores StreamInfo for Context.
type streamInfo struct {
	name       string
	id         string
	streamType StreamType
	metadata   metadata.Metadata
}

func (i *streamInfo) ID() string                  { return i.id }
func (i *streamInfo) Name() string                { return i.name }
func (i *streamInfo) Metadata() metadata.Metadata { return i.metadata }
func (i *streamInfo) StreamType() StreamType      { return i.streamType }
