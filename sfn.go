package yomo

import (
	"context"

	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/serverless"
	"github.com/yomorun/yomo/pkg/id"
)

// StreamFunction defines serverless streaming functions.
type StreamFunction interface {
	// SetObserveDataTags set the data tag list that will be observed
	// Deprecated: use yomo.WithObserveDataTags instead
	SetObserveDataTags(tag ...uint32)
	// SetHandler set the handler function, which accept the raw bytes data and return the tag & response
	SetHandler(fn AsyncHandler) error
	// SetErrorHandler set the error handler function when server error occurs
	SetErrorHandler(fn func(err error))
	// SetPipeHandler set the pipe handler function
	SetPipeHandler(fn PipeHandler) error
	// Connect create a connection to the zipper
	Connect() error
	// Close will close the connection
	Close() error
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
	observeDataTags []uint32     // tag list that will be observed
	asyncHandler    AsyncHandler // user's function which will be invoked when data arrived
	pipeHandler     PipeHandler
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
func (s *streamFunction) SetHandler(h AsyncHandler) error {
	s.asyncHandler = h
	s.client.Logger().Debug("set async handler")
	return nil
}

// SetPipeHandler set the pipe handler function.
func (s *streamFunction) SetPipeHandler(h PipeHandler) error {
	s.pipeHandler = h
	s.client.Logger().Debug("set pipe handler")
	return nil
}

// Connect create a connection to the zipper, when data arrvied, the data will be passed to the
// handler which setted by SetHandler method.
func (s *streamFunction) Connect() error {
	s.client.Logger().Debug("sfn connecting to zipper ...")

	if s.asyncHandler != nil {
		if err := s.asyncHandler.Init(); err != nil {
			s.client.Logger().Debug("failed to init async handler", "err", err)
			return err
		}
	}
	// notify underlying network operations, when data with tag we observed arrived, invoke the func
	s.client.SetDataFrameObserver(func(data *frame.DataFrame) {
		s.client.Logger().Debug("received data frame")
		s.onDataFrame(data)
	})

	if s.pipeHandler != nil {
		// init pipe handler
		if err := s.pipeHandler.Init(); err != nil {
			s.client.Logger().Debug("failed to init pipe handler", "err", err)
			return err
		}

		s.pIn = make(chan []byte)
		s.pOut = make(chan *frame.DataFrame)

		// handle user's pipe function
		go func() {
			s.pipeHandler.Handle(s.pIn, s.pOut)
		}()

		// send user's pipe function outputs to zipper
		go func() {
			for {
				data := <-s.pOut
				if data != nil {
					s.client.Logger().Debug("pipe fn send", "payload_frame", data)

					md, _ := core.NewDefaultMetadata(s.client.ClientID(), false, id.New()).Encode()

					frame := &frame.DataFrame{
						Tag:      data.Tag,
						Metadata: md,
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

// when DataFrame we observed arrived, invoke the user's function
// func (s *streamFunction) onDataFrame(data []byte, metaFrame *frame.MetaFrame) {
func (s *streamFunction) onDataFrame(dataFrame *frame.DataFrame) {
	if s.asyncHandler != nil {
		go func() {
			serverlessCtx := serverless.NewContext(s.client, dataFrame)
			s.asyncHandler.Handle(serverlessCtx)
		}()
	} else if s.pipeHandler != nil {
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
