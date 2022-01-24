package rx

import (
	"context"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/pkg/logger"
)

// Runtime is the Stream Serverless Runtime for RxStream.
type Runtime struct {
	rawBytesChan chan interface{}
	sfn          yomo.StreamFunction
	stream       Stream
}

// NewRuntime creates a new Rx Stream Serverless Runtime.
func NewRuntime(sfn yomo.StreamFunction) *Runtime {
	return &Runtime{
		rawBytesChan: make(chan interface{}),
		sfn:          sfn,
	}
}

// RawByteHandler is the Handler for RawBytes.
func (r *Runtime) RawByteHandler(req []byte) (byte, []byte) {
	go func() {
		r.rawBytesChan <- req
	}()

	// observe the data from RxStream.
	for item := range r.stream.Observe() {
		if item.Error() {
			logger.Errorf("[Rx Handler] Handler got an error, err=%v", item.E)
			continue
		}

		if item.V == nil {
			logger.Warn("[Rx Handler] the returned data is nil.")
			continue
		}

		res, ok := (item.V).(frame.PayloadFrame)
		if !ok {
			logger.Warn("[Rx Handler] the data is not a frame.PayloadFrame, won't send it to YoMo-Zipper.")
			continue
		}

		logger.Infof("[RawByteHandler] Send data with [tag=%#x] to YoMo-Zipper.", res.Tag)
		return res.Tag, res.Carriage
	}

	// return empty data by default, the new data from RxStream will be returned in `Pipe` function.
	return 0x0, nil
}

// PipeHandler processes data sequentially.
func (r *Runtime) PipeHandler(in <-chan []byte, out chan<- *frame.PayloadFrame) {
	go func() {
		for item := range r.stream.Observe() {
			if item.Error() {
				logger.Errorf("[rx PipeHandler] Handler got an error, err=%v", item.E)
				continue
			}

			if item.V == nil {
				logger.Warn("[rx PipeHandler] the returned data is nil.")
				continue
			}

			res, ok := (item.V).(frame.PayloadFrame)
			if !ok {
				logger.Warn("[rx PipeHandler] the data is not a frame.PayloadFrame, won't send it to YoMo-Zipper.")
				continue
			}

			logger.Infof("[rx PipeHandler] Send data with [tag=%#x] to YoMo-Zipper.", res.Tag)
			out <- &res
		}
	}()

	for req := range in {
		r.rawBytesChan <- req
	}
}

// Pipe the RxHandler with RxStream.
func (r *Runtime) Pipe(rxHandler func(rxstream Stream) Stream) {
	fac := NewFactory()
	// create a RxStream from raw bytes channel.
	rxstream := fac.FromChannel(context.Background(), r.rawBytesChan)

	// run RxHandler and get a new RxStream.
	r.stream = rxHandler(rxstream)
}
