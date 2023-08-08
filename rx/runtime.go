package rx

import (
	"context"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/ylog"
	"github.com/yomorun/yomo/serverless"
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
func (r *Runtime) RawByteHandler(ctx serverless.Context) {
	go func() {
		r.rawBytesChan <- ctx.Data()
	}()

	// observe the data from RxStream.
	for item := range r.stream.Observe() {
		if item.Error() {
			ylog.Error("[Rx Handler] Handler got an error", item.E)
			continue
		}

		if item.V == nil {
			ylog.Warn("[Rx Handler] the returned data is nil.")
			continue
		}

		res, ok := (item.V).(frame.DataFrame)
		if !ok {
			ylog.Warn("[Rx Handler] the data is not a frame.PayloadFrame, won't send it to YoMo-Zipper.")
			continue
		}

		ylog.Debug("[RawByteHandler] Send data to YoMo-Zipper.", "tag", res.Tag)
		ctx.Write(res.Tag, res.Payload)
	}
}

// PipeHandler processes data sequentially.
func (r *Runtime) PipeHandler(in <-chan []byte, out chan<- *frame.DataFrame) {
	for {
		select {
		case req := <-in:
			r.rawBytesChan <- req
		case item := <-r.stream.Observe():
			if item.Error() {
				ylog.Error("[rx PipeHandler] Handler got an error", item.E)
				continue
			}

			if item.V == nil {
				ylog.Warn("[rx PipeHandler] the returned data is nil.")
				continue
			}

			res, ok := (item.V).(frame.DataFrame)
			if !ok {
				ylog.Warn("[rx PipeHandler] the data is not a frame.DataFrame, won't send it to YoMo-Zipper.")
				continue
			}

			ylog.Info("[rx PipeHandler] Send data with [tag=%#x] to YoMo-Zipper.", res.Tag)
			out <- &res
		}
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
