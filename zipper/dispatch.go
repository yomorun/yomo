package server

import (
	"context"
	"reflect"
	"time"

	"github.com/reactivex/rxgo/v2"
	"github.com/yomorun/yomo/core/rx"
	"github.com/yomorun/yomo/internal/decoder"
	"github.com/yomorun/yomo/internal/framing"
	"github.com/yomorun/yomo/logger"
)

// DispatcherWithFunc dispatches the input stream to downstreams.
func DispatcherWithFunc(sfns []GetStreamFunc, reader decoder.Reader) rx.Stream {
	stream := rx.NewFactory().FromReader(reader)

	for _, sfn := range sfns {
		stream = mergeStreamFn(stream, sfn)
	}

	return stream
}

// mergeStreamFn sends the stream data to Stream Function and receives the new stream data from it.
func mergeStreamFn(upstream rx.Stream, sfn GetStreamFunc) rx.Stream {
	f := func(ctx context.Context, next chan rxgo.Item) {
		defer close(next)
		response := make(chan framing.Frame)
		observe := upstream.Observe()

		// send the stream to downstream
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case item, ok := <-observe:
					if !ok {
						return
					}
					if item.Error() {
						return
					}

					for {
						name, rw, cancel := sfn()
						frame, ok := item.V.(framing.Frame)
						if !ok {
							logger.Debug("[MergeStreamFunc] the type of item.V is not a Frame", "type", reflect.TypeOf(item.V))
							continue
						}

						if rw == nil {
							logger.Debug("[MergeStreamFunc] the writer of the stream-function is nil", "stream-fn", name)
							// pass the data to next stream function if the curren stream function is nil
							response <- frame
							break
						}

						// send frame to downstream.
						err := rw.Write(frame)
						if err == nil {
							logger.Debug("[MergeStreamFunc] YoMo-Zipper sent frame to Stream Function.", "stream-fn", name, "frame", logger.BytesString(frame.Bytes()))
							break
						} else {
							logger.Error("[MergeStreamFunc] YoMo-Zipper sent frame to Stream Function failed.", "stream-fn", name, "frame", logger.BytesString(frame.Bytes()), "err", err)
							cancel()
						}
					}
				}
			}
		}()

		// receive the response from downstream
		go func() {
			defer close(response)
			for {
				name, rw, _ := sfn()
				if rw == nil {
					time.Sleep(100 * time.Millisecond)
					continue
				}

				frameCh := rw.Read()
				for frame := range frameCh {
					logger.Debug("[MergeStreamFunc] YoMo-Zipper received frame from Stream Function.", "stream-fn", name, "frame", logger.BytesString(frame.Bytes()))
					if frame.Type() == framing.FrameTypePayload {
						response <- frame
					} else if frame.Type() == framing.FrameTypeAck {
						logger.Debug("[MergeStreamFunc] YoMo-Zipper received ACK from Stream Function, will send the data to next Stream Function.", "stream-fn", name)
						// TODO: send data to next Stream Function.
					} else {
						logger.Debug("[MergeStreamFunc] it is not a Payload Frame.", "stream-fn", name, "frame_type", frame.Type())
					}
				}
			}
		}()

		// send response to downstream
		for {
			select {
			case <-ctx.Done():
				return
			case value, ok := <-response:
				if !ok {
					return
				}

				if !rxgo.Of(value).SendContext(ctx, next) {
					return
				}
			}
		}
	}

	return rx.CreateObservable(f)
}
