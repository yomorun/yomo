package yomo

import (
	"context"

	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/core/serverless"
	"github.com/yomorun/yomo/pkg/id"
	"github.com/yomorun/yomo/pkg/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
	"golang.org/x/exp/slog"
)

// StreamFunction defines serverless streaming functions.
type StreamFunction interface {
	// SetObserveDataTags set the data tag list that will be observed
	SetObserveDataTags(tag ...uint32)
	// Init will initialize the stream function
	Init(fn func() error) error
	// SetHandler set the handler function, which accept the raw bytes data and return the tag & response
	SetHandler(fn core.AsyncHandler) error
	// SetErrorHandler set the error handler function when server error occurs
	SetErrorHandler(fn func(err error))
	// SetPipeHandler set the pipe handler function
	SetPipeHandler(fn core.PipeHandler) error
	// Connect create a connection to the zipper
	Connect() error
	// Close will close the connection
	Close() error
	// Wait waits sfn to finish.
	Wait()
}

// NewStreamFunction create a stream function.
func NewStreamFunction(name, zipperAddr string, opts ...SfnOption) StreamFunction {
	clientOpts := make([]core.ClientOption, len(opts))
	for k, v := range opts {
		clientOpts[k] = core.ClientOption(v)
	}
	client := core.NewClient(name, core.ClientTypeStreamFunction, clientOpts...)
	sfn := &streamFunction{
		name:            name,
		zipperAddr:      zipperAddr,
		client:          client,
		observeDataTags: make([]uint32, 0),
	}

	return sfn
}

var _ StreamFunction = &streamFunction{}

// streamFunction implements StreamFunction interface.
type streamFunction struct {
	name            string
	zipperAddr      string
	client          *core.Client
	observeDataTags []uint32          // tag list that will be observed
	fn              core.AsyncHandler // user's function which will be invoked when data arrived
	pfn             core.PipeHandler
	pIn             chan []byte
	pOut            chan *frame.DataFrame
}

// SetObserveDataTags set the data tag list that will be observed.
// Deprecated: use yomo.WithObserveDataTags instead
func (s *streamFunction) SetObserveDataTags(tag ...uint32) {
	s.client.SetObserveDataTags(tag...)
	s.client.Logger().Debug("set sfn observe data tasg", "tags", s.observeDataTags)
}

// SetHandler set the handler function, which accept the raw bytes data and return the tag & response.
func (s *streamFunction) SetHandler(fn core.AsyncHandler) error {
	s.fn = fn
	s.client.Logger().Debug("set async handler")
	return nil
}

func (s *streamFunction) SetPipeHandler(fn core.PipeHandler) error {
	s.pfn = fn
	s.client.Logger().Debug("set pipe handler")
	return nil
}

// Connect create a connection to the zipper, when data arrvied, the data will be passed to the
// handler which setted by SetHandler method.
func (s *streamFunction) Connect() error {
	s.client.Logger().Debug("sfn connecting to zipper ...")
	// notify underlying network operations, when data with tag we observed arrived, invoke the func
	s.client.SetDataFrameObserver(func(data *frame.DataFrame) {
		s.client.Logger().Debug("received data frame")
		s.onDataFrame(data)
	})

	if s.pfn != nil {
		s.pIn = make(chan []byte)
		s.pOut = make(chan *frame.DataFrame)

		// handle user's pipe function
		go func() {
			s.pfn(s.pIn, s.pOut)
		}()

		// send user's pipe function outputs to zipper
		go func() {
			for {
				data := <-s.pOut
				if data != nil {
					s.client.Logger().Debug("pipe fn send", "payload_frame", data)
					md, err := metadata.Decode(data.Metadata)
					if err != nil {
						s.client.Logger().Error("sfn decode metadata error", "err", err)
						break
					}

					newMd, deferFunc := ExtendTraceMetadata(md, s.client.ClientID(), s.client.Name(), s.client.TracerProvider(), s.client.Logger())
					defer deferFunc()

					newMetadata, err := newMd.Encode()
					if err != nil {
						s.client.Logger().Error("sfn encode metadata error", "err", err)
						break
					}
					data.Metadata = newMetadata

					frame := &frame.DataFrame{
						Tag:      data.Tag,
						Metadata: data.Metadata,
						Payload:  data.Payload,
					}

					s.client.WriteFrame(frame)
				}
			}
		}()
	}

	err := s.client.Connect(context.Background(), s.zipperAddr)
	return err
}

// Close will close the connection.
func (s *streamFunction) Close() error {
	if s.pIn != nil {
		close(s.pIn)
	}

	if s.pOut != nil {
		close(s.pOut)
	}

	if s.client != nil {
		if err := s.client.Close(); err != nil {
			s.client.Logger().Error("failed to close sfn", "err", err)
			return err
		}
	}

	return nil
}

// Wait waits sfn to finish.
func (s *streamFunction) Wait() {
	s.client.Wait()
}

// when DataFrame we observed arrived, invoke the user's function
// func (s *streamFunction) onDataFrame(data []byte, metaFrame *frame.MetaFrame) {
func (s *streamFunction) onDataFrame(dataFrame *frame.DataFrame) {
	if s.fn != nil {
		tp := s.client.TracerProvider()
		go func(tp oteltrace.TracerProvider, dataFrame *frame.DataFrame) {
			md, err := metadata.Decode(dataFrame.Metadata)
			if err != nil {
				s.client.Logger().Error("sfn decode metadata error", "err", err)
				return
			}

			newMd, deferFunc := ExtendTraceMetadata(md, s.client.ClientID(), s.client.Name(), s.client.TracerProvider(), s.client.Logger())
			defer deferFunc()

			newMetadata, err := newMd.Encode()
			if err != nil {
				s.client.Logger().Error("sfn encode metadata error", "err", err)
				return
			}
			dataFrame.Metadata = newMetadata
			// TODO: 判断是否是 stream frame
			if core.GetStreamedFromMetadata(newMd) {
				var streamFrame frame.StreamFrame
				err = s.client.FrameStream().Codec().Decode(dataFrame.Payload, &streamFrame)
				if err != nil {
					slog.Error("sfn StreamFrame decode error", "err", err)
					return
				}
				slog.Info("sfn got stream", "stream_frame", streamFrame)
				s.client.Logger().Debug("sfn receive stream frame", "source_id", streamFrame.ClientID, "stream_id", streamFrame.StreamID)

				// TODO: 创建一个流实例,对应 source 的 stream
				sfnStream, err := s.client.RequestStream()
				if err != nil {
					s.client.Logger().Error("sfn request stream error", "err", err)
					return
				}
				s.client.Logger().Debug("sfn create stream", "source_id", streamFrame.ClientID, "stream_id", sfnStream.StreamID())

				// TODO: 将 zipper 上关联的 source stream 与 sfn stream 关联起来
				// sourceStream := s.client.GetStream(streamFrame.StreamID)
				// io.Copy(sfnStream, sourceStream)

			}
			serverlessCtx := serverless.NewContext(s.client, dataFrame)
			s.fn(serverlessCtx)
		}(tp, dataFrame)
	} else if s.pfn != nil {
		data := dataFrame.Payload
		s.client.Logger().Debug("pipe sfn receive", "data_len", len(data), "data", data)
		s.pIn <- data
	} else {
		s.client.Logger().Warn("sfn does not have a handler")
	}
}

// SetErrorHandler set the error handler function when server error occurs
func (s *streamFunction) SetErrorHandler(fn func(err error)) {
	s.client.SetErrorHandler(fn)
}

// Init will initialize the stream function
func (s *streamFunction) Init(fn func() error) error {
	return fn()
}

// ExtendTraceMetadata extends source trace metadata.
func ExtendTraceMetadata(md metadata.M, clientID, name string, tp oteltrace.TracerProvider, logger *slog.Logger) (metadata.M, func()) {
	deferFunc := func() {}
	tid := core.GetTIDFromMetadata(md)
	sid := core.GetSIDFromMetadata(md)
	streamed := core.GetStreamedFromMetadata(md)
	parentTraced := core.GetTracedFromMetadata(md)
	traced := false
	// trace
	if tp != nil {
		// create span
		var span oteltrace.Span
		var err error
		// set parent span, if not traced, use empty string
		if parentTraced {
			span, err = trace.NewSpan(tp, core.ClientTypeStreamFunction.String(), name, tid, sid)
		} else {
			span, err = trace.NewSpan(tp, core.ClientTypeStreamFunction.String(), name, "", "")
		}
		if err != nil {
			logger.Error("sfn trace error", "err", err)
		} else {
			deferFunc = func() { span.End() }
			tid = span.SpanContext().TraceID().String()
			sid = span.SpanContext().SpanID().String()
			traced = true
		}
	}
	if tid == "" {
		logger.Debug("sfn create new tid")
		tid = id.TID()
	}
	if sid == "" || !traced {
		logger.Debug("sfn create new sid")
		sid = id.SID()
	}
	// reallocate metadata with new TID and SID
	core.SetTIDToMetadata(md, tid)
	core.SetSIDToMetadata(md, sid)
	core.SetTracedToMetadata(md, traced)
	core.SetStreamedToMetadata(md, streamed)

	logger.Debug("sfn metadata", "tid", tid, "sid", sid, "parentTraced", parentTraced, "traced", traced, "streamed", streamed)

	return md, deferFunc
}
