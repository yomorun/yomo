package yomo

import (
	"context"
	"errors"

	"github.com/robfig/cron/v3"

	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/core/serverless"
	"github.com/yomorun/yomo/pkg/id"
	"github.com/yomorun/yomo/pkg/trace"
	"go.opentelemetry.io/otel/attribute"
)

// StreamFunction defines serverless streaming functions.
type StreamFunction interface {
	// SetWantedTarget sets target for sfn that to receive data carrying the same target.
	// This function is optional and it should be called before Connect().
	SetWantedTarget(string)
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
	// SetCronHandler set the cron handler function.
	//  Examples:
	//  sfn.SetCronHandler("0 30 * * * *", func(ctx serverless.CronContext) {})
	//  sfn.SetCronHandler("@hourly",      func(ctx serverless.CronContext) {})
	//  sfn.SetCronHandler("@every 1h30m", func(ctx serverless.CronContext) {})
	// more spec style see: https://pkg.go.dev/github.com/robfig/cron#hdr-Usage
	SetCronHandler(spec string, fn core.CronHandler) error
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

	client := core.NewClient(name, zipperAddr, core.ClientTypeStreamFunction, clientOpts...)

	client.Logger = client.Logger.With(
		"component", core.ClientTypeStreamFunction.String(),
		"sfn_id", client.ClientID(),
		"sfn_name", client.Name(),
		"zipper_addr", zipperAddr,
	)

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
	cronSpec        string
	cronFn          core.CronHandler
	cron            *cron.Cron
	pOut            chan *frame.DataFrame
}

func (s *streamFunction) SetWantedTarget(target string) {
	s.client.SetWantedTarget(target)
}

// SetObserveDataTags set the data tag list that will be observed.
func (s *streamFunction) SetObserveDataTags(tag ...uint32) {
	s.observeDataTags = tag
	s.client.SetObserveDataTags(tag...)
	s.client.Logger.Debug("set sfn observe data tasg", "tags", s.observeDataTags)
}

// SetHandler set the handler function, which accept the raw bytes data and return the tag & response.
func (s *streamFunction) SetHandler(fn core.AsyncHandler) error {
	s.fn = fn
	s.client.Logger.Debug("set async handler")
	return nil
}

func (s *streamFunction) SetCronHandler(cronSpec string, fn core.CronHandler) error {
	s.cronSpec = cronSpec
	s.cronFn = fn
	s.client.Logger.Debug("set cron handler")
	return nil
}

func (s *streamFunction) SetPipeHandler(fn core.PipeHandler) error {
	s.pfn = fn
	s.client.Logger.Debug("set pipe handler")
	return nil
}

// Connect create a connection to the zipper, when data arrvied, the data will be passed to the
// handler set by SetHandler method.
func (s *streamFunction) Connect() error {
	hasCron := s.cronFn != nil && s.cronSpec != ""
	if hasCron {
		s.cron = cron.New()
		s.cron.AddFunc(s.cronSpec, func() {
			md := core.NewMetadata(s.client.ClientID(), id.New())
			// add trace
			tracer := trace.NewTracer("StreamFunction")
			span := tracer.Start(md, s.name)
			defer tracer.End(md, span, attribute.String("sfn_handler_type", "corn_handler"))

			cronCtx := serverless.NewCronContext(s.client, md)
			s.cronFn(cronCtx)
		})
		s.cron.Start()
	}

	if len(s.observeDataTags) == 0 && !hasCron {
		return errors.New("streamFunction cannot observe data because the required tag has not been set")
	}

	s.client.Logger.Debug("sfn connecting to zipper ...")
	// notify underlying network operations, when data with tag we observed arrived, invoke the func
	s.client.SetDataFrameObserver(func(data *frame.DataFrame) {
		s.client.Logger.Debug("received data frame")
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
					s.client.Logger.Debug("pipe fn send", "payload_frame", data)
					md, err := metadata.Decode(data.Metadata)
					if err != nil {
						s.client.Logger.Error("sfn decode metadata error", "err", err)
						break
					}

					// add trace
					tracer := trace.NewTracer("StreamFunction")
					span := tracer.Start(md, s.name)
					defer tracer.End(
						md,
						span,
						attribute.String("sfn_handler_type", "pipe_handler"),
						attribute.Int("recv_data_tag", int(data.Tag)),
						attribute.Int("recv_data_len", len(data.Payload)),
					)

					rawMd, err := md.Encode()
					if err != nil {
						s.client.Logger.Error("sfn encode metadata error", "err", err)
						break
					}
					data.Metadata = rawMd

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

	err := s.client.Connect(context.Background())
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

	if s.cron != nil {
		s.cron.Stop()
	}

	if s.client != nil {
		if err := s.client.Close(); err != nil {
			s.client.Logger.Error("failed to close sfn", "err", err)
			return err
		}
	}

	trace.ShutdownTracerProvider()

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
		go func(dataFrame *frame.DataFrame) {
			md, err := metadata.Decode(dataFrame.Metadata)
			if err != nil {
				s.client.Logger.Error("sfn decode metadata error", "err", err)
				return
			}

			// add trace
			tracer := trace.NewTracer("StreamFunction")
			span := tracer.Start(md, s.name)
			defer tracer.End(
				md,
				span,
				attribute.String("sfn_handler_type", "async_handler"),
				attribute.Int("recv_data_tag", int(dataFrame.Tag)),
				attribute.Int("recv_data_len", len(dataFrame.Payload)),
			)

			serverlessCtx := serverless.NewContext(s.client, dataFrame.Tag, md, dataFrame.Payload)
			s.fn(serverlessCtx)
		}(dataFrame)
	} else if s.pfn != nil {
		data := dataFrame.Payload
		s.client.Logger.Debug("pipe sfn receive", "data_len", len(data), "data", data)
		s.pIn <- data
	} else {
		s.client.Logger.Warn("sfn does not have a handler")
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
