package yomo

import (
	"context"

	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/frame"
)

// StreamFunction defines serverless streaming functions.
type StreamFunction interface {
	// SetObserveDataTags set the data tag list that will be observed
	// Deprecated: use yomo.WithObserveDataTags instead
	SetObserveDataTags(tag ...frame.Tag)
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
	// Send a data to zipper.
	Write(tag frame.Tag, carriage []byte) error
}

// NewStreamFunction create a stream function.
func NewStreamFunction(name string, opts ...Option) StreamFunction {
	options := newOptions(opts...)
	client := core.NewClient(name, core.ClientTypeStreamFunction, options.clientOptions...)
	sfn := &streamFunction{
		name:            name,
		zipperEndpoint:  options.zipperAddr,
		client:          client,
		observeDataTags: make([]frame.Tag, 0),
	}

	return sfn
}

var _ StreamFunction = &streamFunction{}

// streamFunction implements StreamFunction interface.
type streamFunction struct {
	name            string
	zipperEndpoint  string
	client          *core.Client
	observeDataTags []frame.Tag       // tag list that will be observed
	fn              core.AsyncHandler // user's function which will be invoked when data arrived
	pfn             core.PipeHandler
	pIn             chan []byte
	pOut            chan *frame.PayloadFrame
}

// SetObserveDataTags set the data tag list that will be observed.
// Deprecated: use yomo.WithObserveDataTags instead
func (s *streamFunction) SetObserveDataTags(tag ...frame.Tag) {
	s.client.SetObserveDataTags(tag...)
	s.client.Logger().Debug("sSetObserveDataTag", "tags", s.observeDataTags)
}

// SetHandler set the handler function, which accept the raw bytes data and return the tag & response.
func (s *streamFunction) SetHandler(fn core.AsyncHandler) error {
	s.fn = fn
	s.client.Logger().Debug("SetHandler", "handler", s.fn)
	return nil
}

func (s *streamFunction) SetPipeHandler(fn core.PipeHandler) error {
	s.pfn = fn
	s.client.Logger().Debug("SetHandler", "pipe_handler", s.pfn)
	return nil
}

// Connect create a connection to the zipper, when data arrvied, the data will be passed to the
// handler which setted by SetHandler method.
func (s *streamFunction) Connect() error {
	s.client.Logger().Debug("Connect")
	// notify underlying network operations, when data with tag we observed arrived, invoke the func
	s.client.SetDataFrameObserver(func(data *frame.DataFrame) {
		s.client.Logger().Debug("receive DataFrame", "data_frame", data.String())
		s.onDataFrame(data.GetCarriage(), data.GetMetaFrame())
	})

	if s.pfn != nil {
		s.pIn = make(chan []byte)
		s.pOut = make(chan *frame.PayloadFrame)

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
					frame := frame.NewDataFrame()
					// todo: frame.SetTransactionID
					frame.SetCarriage(data.Tag, data.Carriage)
					s.client.WriteFrame(frame)
				}
			}
		}()
	}

	err := s.client.Connect(context.Background(), s.zipperEndpoint)
	if err != nil {
		s.client.Logger().Error("Connect error", err)
	}
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
			s.client.Logger().Error("Close error", err)
			return err
		}
	}

	return nil
}

// when DataFrame we observed arrived, invoke the user's function
func (s *streamFunction) onDataFrame(data []byte, metaFrame *frame.MetaFrame) {
	s.client.Logger().Debug("onDataFrame")

	if s.fn != nil {
		go func() {
			// invoke serverless
			tag, resp := s.fn(data)
			// if resp is not nil, means the user's function has returned something, we should send it to the zipper
			if len(resp) != 0 {
				// build a DataFrame
				// TODO: seems we should implement a DeepCopy() of MetaFrame in the future
				frame := frame.NewDataFrame()
				// reuse transactionID
				frame.SetTransactionID(metaFrame.TransactionID())
				// reuse sourceID
				frame.SetSourceID(metaFrame.SourceID())
				frame.SetCarriage(tag, resp)
				s.client.Logger().Debug("start WriteFrame()", "resp", resp)
				s.client.WriteFrame(frame)
			}
		}()
	} else if s.pfn != nil {
		s.client.Logger().Debug("pipe fn receive", "data_len", len(data), "data", data)
		s.pIn <- data
	} else {
		s.client.Logger().Warn("StreamFunction is nil")
	}
}

// Send a DataFrame to zipper.
func (s *streamFunction) Write(tag frame.Tag, carriage []byte) error {
	frame := frame.NewDataFrame()
	frame.SetCarriage(tag, carriage)
	return s.client.WriteFrame(frame)
}

// SetErrorHandler set the error handler function when server error occurs
func (s *streamFunction) SetErrorHandler(fn func(err error)) {
	s.client.SetErrorHandler(fn)
}
