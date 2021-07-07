package server

import (
	"context"
	"io"

	"github.com/reactivex/rxgo/v2"
	"github.com/yomorun/yomo/internal/decoder"
	"github.com/yomorun/yomo/logger"
	"github.com/yomorun/yomo/quic"
	"github.com/yomorun/yomo/rx"
)

// DispatcherWithFunc dispatches the input stream to downstreams.
func DispatcherWithFunc(sfns []GetStreamFunc, reader io.Reader) rx.Stream {
	stream := rx.FromReader(reader)

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
		streamReady := make(chan bool)
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
							rw, cancel := sfn()
							data := item.V.([]byte)
							if rw == nil {
								// pass the data to next stream function if the curren stream function is nil
								response <- data
								break
							} else {
								streamReady <- true
								_, err := rw.Write(data)
								if err == nil {
									logger.Debug("[MergeStreamFunc] YoMo-Server sent frame to Stream Function.", "frame", logger.BytesString(data))
									break
								} else {
									logger.Error("[MergeStreamFunc] YoMo-Server sent frame to Stream Function failed.", "frame", logger.BytesString(data), "err", err)
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
			defer close(streamReady)

			for {
				select {
				case ready, ok := <-streamReady:
					if !ready || !ok {
						return
					}

					go func() {
						rw, cancel := sfn()
						if rw != nil {
							fd := decoder.NewFrameDecoder(rw)
							buf, err := fd.Read(false)
							if err != nil && err != io.EOF {
								if err.Error() != quic.ErrConnectionClosed {
									logger.Error("[MergeStreamFunc] YoMo-Server received frame from Stream Function failed.", "err", err)
								}
								cancel()
							} else {
								logger.Debug("[MergeStreamFunc] YoMo-Server received frame from Stream Function.", "frame", logger.BytesString(buf))
								response <- buf
							}
						}
					}()
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
