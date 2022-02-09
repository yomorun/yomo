package yomo

import (
	"context"
	// "fmt"

	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/pkg/logger"
)

const (
	streamFunctionLogPrefix = "\033[31m[yomo:sfn]\033[0m "
)

// StreamFunction defines serverless streaming functions.
type StreamFunction interface {
	// SetObserveDataTag set the data tag list that will be observed
	SetObserveDataTag(id ...byte)
	// SetHandler set the handler function, which accept the raw bytes data and return the tag & response
	SetHandler(fn core.AsyncHandler) error
	// SetPipeHandler set the pipe handler function
	SetPipeHandler(fn core.PipeHandler) error
	// Connect create a connection to the zipper
	Connect() error
	// Close will close the connection
	Close() error
	// Write will write data with specified tag.
	Write(tag byte, carriage []byte) error
	// WriteDataFrame will write data frame to zipper.
	WriteDataFrame(f *frame.DataFrame) error
}

// NewStreamFunction create a stream function.
func NewStreamFunction(name string, opts ...Option) StreamFunction {
	options := NewOptions(opts...)
	client := core.NewClient(name, core.ClientTypeStreamFunction, options.ClientOptions...)
	sfn := &streamFunction{
		name:           name,
		zipperEndpoint: options.ZipperAddr,
		client:         client,
		observed:       make([]byte, 0),
	}

	return sfn
}

var _ StreamFunction = &streamFunction{}

// streamFunction implements StreamFunction interface.
type streamFunction struct {
	name           string
	zipperEndpoint string
	client         *core.Client
	observed       []byte            // ID list that will be observed
	fn             core.AsyncHandler // user's function which will be invoked when data arrived
	pfn            core.PipeHandler
	pIn            chan []byte
	pOut           chan *frame.DataFrame
}

// SetObserveDataTag set the data tag list that will be observed.
func (s *streamFunction) SetObserveDataTag(id ...byte) {
	s.observed = append(s.observed, id...)
	logger.Debugf("%sSetObserveDataTag(%v)", streamFunctionLogPrefix, s.observed)
}

// SetHandler set the handler function, which accept the raw bytes data and return the tag & response.
func (s *streamFunction) SetHandler(fn core.AsyncHandler) error {
	s.fn = fn
	logger.Debugf("%sSetHandler(%v)", streamFunctionLogPrefix, s.fn)
	return nil
}

func (s *streamFunction) SetPipeHandler(fn core.PipeHandler) error {
	s.pfn = fn
	logger.Debugf("%sSetHandler(%v)", streamFunctionLogPrefix, s.fn)
	return nil
}

// Connect create a connection to the zipper, when data arrvied, the data will be passed to the
// handler which setted by SetHandler method.
func (s *streamFunction) Connect() error {
	logger.Debugf("%s Connect()", streamFunctionLogPrefix)
	// notify underlying network operations, when data with tag we observed arrived, invoke the func
	s.client.SetDataFrameObserver(func(data *frame.DataFrame) {
		logger.Debugf("%sreceive DataFrame, tag=%# x, carraige=%# x", streamFunctionLogPrefix, data.Tag(), data.GetCarriage())
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
					logger.Debugf("%spipe fn send: ts=%s, tag=%#x, data=%# x", streamFunctionLogPrefix, data.GetMetaFrame().Timestamp(), data.Tag(), data.GetCarriage())
					s.client.WriteFrame(data)
				}
			}
		}()
	}

	err := s.client.Connect(context.Background(), s.zipperEndpoint)
	if err != nil {
		logger.Errorf("%sConnect() error: %s", streamFunctionLogPrefix, err)
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
			logger.Errorf("%sClose(): %v", err)
			return err
		}
	}

	return nil
}

// when DataFrame we observed arrived, invoke the user's function
func (s *streamFunction) onDataFrame(req *frame.DataFrame) {
	logger.Infof("%sonDataFrame ->[%s]", streamFunctionLogPrefix, s.name)

	if s.fn != nil {
		go func() {
			logger.Debugf("%sexecute-start fn: req=%# x", streamFunctionLogPrefix, req)
			// invoke serverless
			tag, resp := s.fn(req.GetCarriage())
			logger.Debugf("%sexecute-done fn: tag=%#x, resp=%# x", streamFunctionLogPrefix, tag, resp)
			// if resp is not nil, means the user's function has returned something, we should send it to the zipper
			if len(resp) != 0 {
				logger.Debugf("%sstart WriteFrame(): tag=%#x, res=%# x", streamFunctionLogPrefix, tag, resp)
				res := frame.NewDataFrame()
				res.SetCarriage(tag, resp)
				s.client.WriteFrame(res)
			}
		}()
	} else if s.pfn != nil {
		logger.Debugf("%spipe fn receive: req=%# x", streamFunctionLogPrefix, req)
		s.pIn <- req.GetCarriage()
	} else {
		logger.Warnf("%sStreamFunction is nil", streamFunctionLogPrefix)
	}
}

// Write will write data with specified tag.
func (s *streamFunction) Write(tag byte, carriage []byte) error {
	f := frame.NewDataFrame()
	f.SetCarriage(tag, carriage)
	return s.WriteDataFrame(f)
}

// WriteDataFrame will write data frame to zipper.
func (s *streamFunction) WriteDataFrame(f *frame.DataFrame) error {
	return s.client.WriteFrame(f)
}
