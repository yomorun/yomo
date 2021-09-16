package serverless

import (
	"context"
	"os"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/cli/pkg/log"
	"github.com/yomorun/yomo/core/rx"
	"github.com/yomorun/yomo/internal/frame"
)

const bufferSize = 100

// RxRuntime is the Stream Serverless Runtime for RxStream.
type RxRuntime struct {
	rawBytesChan chan interface{}
	sfn          yomo.StreamFunction
}

// NewRxRuntime creates a new Rx Stream Serverless Runtime.
func NewRxRuntime(sfn yomo.StreamFunction) *RxRuntime {
	return &RxRuntime{
		rawBytesChan: make(chan interface{}, bufferSize),
		sfn:          sfn,
	}
}

// RawByteHandler is the Handler for RawBytes.
func (r *RxRuntime) RawByteHandler(data []byte) (byte, []byte) {
	// set raw bytes to channel.
	go func() {
		r.rawBytesChan <- data
	}()

	// return empty data by default, the new data from RxStream will be returned in `Pipe` function.
	return 0x0, nil
}

// Pipe the RxHandler with RxStream.
func (r *RxRuntime) Pipe(rxHandler func(rxstream rx.Stream) rx.Stream) {
	fac := rx.NewFactory()
	// create a RxStream from raw bytes channel.
	rxstream := fac.FromChannel(context.Background(), r.rawBytesChan)

	// run RxHandler and get a new RxStream.
	stream := rxHandler(rxstream)

	// observe the data from RxStream.
	for item := range stream.Observe() {
		if item.Error() {
			log.FailureStatusEvent(os.Stdout, "[Rx Handler] Handler got an error, err=%v", item.E)
			continue
		}

		if item.V == nil {
			log.InfoStatusEvent(os.Stdout, "[Rx Handler] the returned data is nil.")
			continue
		}

		dataFrame, ok := (item.V).(*frame.DataFrame)
		if !ok {
			log.InfoStatusEvent(os.Stdout, "[Rx Handler] the data is not a *DataFrame, won't send it to YoMo-Zipper.")
			continue
		}

		// send data to YoMo-Zipper.
		err := r.sfn.Send(dataFrame)
		if err != nil {
			log.FailureStatusEvent(os.Stdout, "[Rx Handler] ❌ Send data to YoMo-Zipper failed, err=%v", err)
		} else {
			log.InfoStatusEvent(os.Stdout, "[Rx Handler] Send data to YoMo-Zipper.")
		}
	}
}
