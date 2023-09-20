package core

import (
	"context"
	"errors"
	"io"

	"github.com/quic-go/quic-go"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
)

// StreamInfo holds the information of DataStream.
type StreamInfo interface {
	// Name returns the name of the stream, which is set by clients.
	Name() string
	// ID represents the dataStream ID, the ID is an unique string.
	ID() string
	// ClientType represents client type (Source | SFN | UpstreamZipper).
	ClientType() ClientType
	// Metadata returns the extra info of the application.
	// The metadata is a merged set of data from both the handshake and authentication processes.
	Metadata() metadata.M
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
	clientType ClientType
	metadata   metadata.M
	observed   []frame.Tag
	stream     *FrameStream

	serverController *ServerControlStream
	clientSignalChan <-chan frame.Frame
}

// newDataStream constructures dataStream.
func newDataStream(
	name string,
	id string,
	clientType ClientType,
	metadata metadata.M,
	observed []frame.Tag,
	stream *FrameStream,
	serverController *ServerControlStream,
	clientSignalChan <-chan frame.Frame,
) DataStream {
	return &dataStream{
		name:       name,
		id:         id,
		clientType: clientType,
		metadata:   metadata,
		observed:   observed,
		stream:     stream,

		serverController: serverController,
		clientSignalChan: clientSignalChan,
	}
}

// DataStream implements.
func (s *dataStream) Context() context.Context     { return s.stream.Context() }
func (s *dataStream) ID() string                   { return s.id }
func (s *dataStream) Name() string                 { return s.name }
func (s *dataStream) Metadata() metadata.M         { return s.metadata }
func (s *dataStream) ClientType() ClientType       { return s.clientType }
func (s *dataStream) ObserveDataTags() []frame.Tag { return s.observed }
func (s *dataStream) Close() error                 { return s.stream.Close() }

func (s *dataStream) WriteFrame(f frame.Frame) error {
	if err := readErrorFromController(s.stream, s.clientSignalChan); err != nil {
		return err
	}
	return s.stream.WriteFrame(f)
}

func (s *dataStream) ReadFrame() (frame.Frame, error) {
	type outCh struct {
		frame frame.Frame
		err   error
	}

	out := make(chan outCh)
	go func() {
		for signal := range s.clientSignalChan {
			switch ff := signal.(type) {
			case *frame.GoawayFrame:
				_ = s.stream.Close()
				out <- outCh{
					frame: nil,
					err:   NewErrControllSignal(ff.Message),
				}
				return
			case *frame.RejectedFrame:
				_ = s.stream.Close()
				out <- outCh{
					frame: nil,
					err:   NewErrControllSignal(ff.Message),
				}
				return
			}
		}
	}()
	go func() {
		f, err := s.stream.ReadFrame()
		// return EOF if server-side control stream has been closed.
		if IsYomoCloseError(err) {
			out <- outCh{
				frame: nil,
				err:   io.EOF,
			}
			return
		}
		out <- outCh{
			frame: f,
			err:   err,
		}
	}()
	result := <-out

	return result.frame, result.err
}

// ContextReadWriteCloser represents a stream which its lifecycle managed by context.
// The context should be closed when the stream is closed.
type ContextReadWriteCloser interface {
	// Context returns the context which manage the lifecycle of stream.
	Context() context.Context
	// The stream.
	io.ReadWriteCloser
}

// ErrControllSignal represents the error of controll signal.
type ErrControllSignal struct {
	errString string
}

// NewErrControllSignal constructs ErrControllSignal.
func NewErrControllSignal(errString string) *ErrControllSignal {
	return &ErrControllSignal{
		errString: errString,
	}
}

// Error implements error interface.
func (e *ErrControllSignal) Error() string {
	return e.errString
}

// readErrorFromController try to read error from controller,if there readan error
// from the controller, the stream read function will return the error and the stream will be closed.
func readErrorFromController(closer io.Closer, ch <-chan frame.Frame) error {
	select {
	case ex := <-ch:
		switch ff := ex.(type) {
		case *frame.GoawayFrame:
			_ = closer.Close()
			return NewErrControllSignal(ff.Message)
		case *frame.RejectedFrame:
			_ = closer.Close()
			return NewErrControllSignal(ff.Message)
		}
	default:
	}
	return nil
}

// IsYomoCloseError checks if the error is yomo close error.
func IsYomoCloseError(err error) bool {
	qerr := new(quic.ApplicationError)
	return errors.As(err, &qerr) && qerr.ErrorCode == YomoCloseErrorCode
}
