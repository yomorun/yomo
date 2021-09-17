package rx

import (
	"context"
	"os"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/cli/pkg/log"
)

const bufferSize = 100

// Runtime is the Stream Serverless Runtime for RxStream.
type Runtime struct {
	rawBytesChan chan interface{}
	sfn          yomo.StreamFunction
}

// NewRuntime creates a new Rx Stream Serverless Runtime.
func NewRuntime(sfn yomo.StreamFunction) *Runtime {
	return &Runtime{
		rawBytesChan: make(chan interface{}, bufferSize),
		sfn:          sfn,
	}
}

// RawByteHandler is the Handler for RawBytes.
func (r *Runtime) RawByteHandler(data []byte) (byte, []byte) {
	// set raw bytes to channel.
	go func() {
		r.rawBytesChan <- data
	}()

	// return empty data by default, the new data from RxStream will be returned in `Pipe` function.
	return 0x0, nil
}

// Pipe the RxHandler with RxStream.
func (r *Runtime) Pipe(rxHandler func(rxstream Stream) Stream) {
	fac := NewFactory()
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

		data, ok := (item.V).(BytesWithDataID)
		if !ok {
			log.InfoStatusEvent(os.Stdout, "[Rx Handler] the data is not a BytesWithDataID, won't send it to YoMo-Zipper.")
			continue
		}

		// send data to YoMo-Zipper.
		err := r.sfn.Write(data.DataID, data.Bytes)
		if err != nil {
			log.FailureStatusEvent(os.Stdout, "[Rx Handler] ❌ Send data to YoMo-Zipper failed, err=%v", err)
		} else {
			log.InfoStatusEvent(os.Stdout, "[Rx Handler] Send data to YoMo-Zipper.")
		}
	}
}
