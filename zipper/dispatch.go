package server

import (
	"context"
	"io"

	"github.com/reactivex/rxgo/v2"
	"github.com/yomorun/yomo/core/quic"
	"github.com/yomorun/yomo/core/rx"
	"github.com/yomorun/yomo/internal/decoder"
	"github.com/yomorun/yomo/internal/framing"
	"github.com/yomorun/yomo/logger"
)

// DispatcherWithFunc dispatches the input stream to downstreams.
func DispatcherWithFunc(sfns []GetStreamFunc, reader io.Reader) rx.Stream {
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
		response := make(chan []byte)
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
					} else {
						for {
							name, rw, cancel := sfn()
							data := item.V.([]byte)
							if rw == nil {
								logger.Debug("[MergeStreamFunc] the writer of the stream-function is nil", "stream-fn", name)
								// pass the data to next stream function if the curren stream function is nil
								response <- data
								break
							} else {
								_, err := rw.Write(data)
								if err == nil {
									logger.Debug("[MergeStreamFunc] YoMo-Zipper sent frame to Stream Function.", "stream-fn", name, "frame", logger.BytesString(data))
									break
								} else {
									logger.Error("[MergeStreamFunc] YoMo-Zipper sent frame to Stream Function failed.", "stream-fn", name, "frame", logger.BytesString(data), "err", err)
									cancel()
								}
							}
						}
					}
				}
			}
		}()

		// receive the response from downstream
		go func() {
			defer close(response)
			for {
				name, rw, cancel := sfn()
				if rw != nil {
					fd := decoder.NewFrameDecoder(rw)
					buf, err := fd.Read(true)
					if err != nil && err != io.EOF {
						if err.Error() != quic.ErrConnectionClosed {
							logger.Error("[MergeStreamFunc] YoMo-Zipper received frame from Stream Function failed.", "stream-fn", name, "err", err)
						}
						cancel()
					} else {
						logger.Debug("[MergeStreamFunc] YoMo-Zipper received frame from Stream Function.", "stream-fn", name, "frame", logger.BytesString(buf))
						f, err := framing.FromRawBytes(buf)
						if err != nil {
							logger.Error("[MergeStreamFunc] framing.FromRawBytes failed:", "stream-fn", name, "err", err)
						} else if f.Type() == framing.FrameTypePayload {
							response <- f.Bytes()
						} else {
							logger.Debug("[MergeStreamFunc] it is not a Payload Frame.", "stream-fn", name, "frame_type", f.Type())
						}
					}
				}
			}
		}()

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
