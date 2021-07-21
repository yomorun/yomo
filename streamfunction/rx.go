package streamfunction

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/reactivex/rxgo/v2"
	"github.com/yomorun/yomo/core/rx"
	"github.com/yomorun/yomo/logger"
)

// StreamfnRx is an interface for the internal Rx Operators in Stream Function.
type StreamfnRx interface {
	// AppendNewDataToRawStream appends new data into raw stream.
	AppendNewDataToRawStream(rawStream rx.Stream, fnStream rx.Stream) rx.Stream

	// GetAppendedStream gets the stream which appending the new data.
	GetAppendedStream(readers chan io.Reader, handler func(rxstream rx.Stream) rx.Stream) rx.Stream
}

type rxImpl struct {
	rxFactory rx.Factory
}

func newStreamFnRx() StreamfnRx {
	return &rxImpl{
		rxFactory: rx.NewFactory(),
	}
}

// AppendNewDataToRawStream appends new data into raw stream.
// 1. receive `raw stream` from YoMo-Zipper.
// 2. receive a new `fn stream` after running `Handler` function.
// 3. append the data in `fn stream` to `raw stream`.
func (r *rxImpl) AppendNewDataToRawStream(rawStream rx.Stream, fnStream rx.Stream) rx.Stream {
	opts := []rxgo.Option{
		rxgo.WithErrorStrategy(rxgo.ContinueOnError),
	}

	f := func(ctx context.Context, next chan rxgo.Item) {
		defer close(next)
		rawData := rawStream.Observe(opts...)
		newData := fnStream.Observe(opts...)
		mutex := sync.Mutex{}
		buf := make([]byte, 0)

		// receive data from raw stream.
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case rawItem, ok := <-rawData:
					if !ok {
						return
					}

					if rawItem.Error() {
						logger.Debug("[Stream Function Client] The raw data has an error.", "err", rawItem.E)
						continue
					}

					rawBuf, ok := (rawItem.V).([]byte)
					if !ok {
						logger.Debug("[Stream Function Client] The type of raw data is not []byte.", "rawData", rawItem.V)
						continue
					}
					// append data to buf.
					mutex.Lock()
					buf = append(buf, rawBuf...)
					mutex.Unlock()
				}
			}
		}()

		// receive new data from the stream by `Handler` function.
		for {
			select {
			case <-ctx.Done():
				return
			case newItem, ok := <-newData:
				if !ok {
					return
				}

				// Correct steps: receive raw data first, then run `Handler` function and get new data.
				for len(buf) == 0 {
					logger.Debug("[Stream Function Client] didn't receive raw data from YoMo-Zipper, skip this new data", "newData", newItem.V)
					continue
				}

				mutex.Lock()

				if newItem.Error() {
					fmt.Println("test error")
					logger.Debug("[Stream Function Client] The new data has an error.", "err", newItem.E)
				} else {
					newBuf, ok := (newItem.V).([]byte)
					if !ok {
						logger.Debug("[Stream Function Client] The type of new data is not []byte, won't append it to raw stream.", "newData", newItem.V)
					} else {
						logger.Debug("[Stream Function Client] Append the new data into raw data.", "rawData", logger.BytesString(buf), "newData", logger.BytesString(newBuf))
						// append new data to buf.
						buf = append(buf, newBuf...)
					}
				}

				if len(buf) > 0 {
					// send data to YoMo-Zipper.
					rx.Of(buf).SendContext(ctx, next)
					// reset
					buf = make([]byte, 0)
				}

				mutex.Unlock()
			}
		}
	}

	return rx.CreateObservable(f, opts...)
}

// GetAppendedStream gets the stream which appending the new data.
func (r *rxImpl) GetAppendedStream(readers chan io.Reader, handler func(rxstream rx.Stream) rx.Stream) rx.Stream {
	// create a RxStream from io.Reader with decoder.
	rxStream := r.rxFactory.FromReaderWithDecoder(readers)
	// create a RawStream from the raw bytes in RxStream.
	rawStream := rxStream.RawBytes()
	// create a new stream by running the `Handler` function.
	fnStream := handler(rxStream)

	// https://github.com/ReactiveX/RxGo#connectable-observable
	// rxstream begins to emit items.
	rxStream.Connect(context.Background())

	// zip RawStream and the new stream from 'Handler' function.
	return r.AppendNewDataToRawStream(rawStream, fnStream)
}
