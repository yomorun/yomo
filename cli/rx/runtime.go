package rx

import (
	"context"
	"os"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/cli/pkg/log"
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
func (r *Runtime) RawByteHandler(data []byte) (byte, []byte) {
	go func() {
		r.rawBytesChan <- data
	}()

	// observe the data from RxStream.
	for item := range r.stream.Observe() {
		if item.Error() {
			log.FailureStatusEvent(os.Stdout, "[Rx Handler] Handler got an error, err=%v", item.E)
			continue
		}

		if item.V == nil {
			log.InfoStatusEvent(os.Stdout, "[Rx Handler] the returned data is nil.")
			continue
		}

		data, ok := (item.V).(BytesWithDataID)
		if !ok {
			log.InfoStatusEvent(os.Stdout, "[Rx Handler] the data is not a BytesWithDataID, won't send it to YoMo-Zipper.")
			continue
		}

		log.InfoStatusEvent(os.Stdout, "[RawByteHandler] Send data with [tag=%#x] to YoMo-Zipper.", data.DataID)
		return data.DataID, data.Bytes
	}

	// return empty data by default, the new data from RxStream will be returned in `Pipe` function.
	return 0x0, nil
}

// Pipe the RxHandler with RxStream.
func (r *Runtime) Pipe(rxHandler func(rxstream Stream) Stream) {
	fac := NewFactory()
	// create a RxStream from raw bytes channel.
	rxstream := fac.FromChannel(context.Background(), r.rawBytesChan)

	// run RxHandler and get a new RxStream.
	r.stream = rxHandler(rxstream)
}
