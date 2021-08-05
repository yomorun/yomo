package server

import (
	"context"
	"reflect"
	"runtime"
	"time"

	"github.com/reactivex/rxgo/v2"
	"github.com/yomorun/yomo/core/quic"
	"github.com/yomorun/yomo/core/rx"
	"github.com/yomorun/yomo/internal/decoder"
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
		response := make(chan []byte)
		observe := upstream.Observe()

		// send the stream to downstream
		go sendObservedDataToStreamFn(ctx, sfn, observe, response)

		// receive the response from downstream
		go receiveResponseFromStreamFn(sfn, response)

		// send response to downstream
		sendResponseToStreamFn(ctx, next, response)
	}

	return rx.CreateObservable(f)
}

func sendObservedDataToStreamFn(ctx context.Context, sfn GetStreamFunc, observe <-chan rxgo.Item, response chan []byte) {
	for {
		select {
		case <-ctx.Done():
			return
		case item, ok := <-observe:
			if !ok {
				return
			}
			if item.Error() {
				logger.Error("[MergeStreamFunc] observe data from upstream failed.", "err", item.E)
				return
			}

		LOOP_READ_STREAM:
			for {
				name, session, cancel := sfn()
				data, ok := item.V.([]byte)
				if !ok {
					logger.Debug("[MergeStreamFunc] the type of item.V is not a []byte", "type", reflect.TypeOf(item.V))
					continue
				}

				if session == nil {
					logger.Debug("[MergeStreamFunc] the session of the stream-function is nil", "stream-fn", name)
					// pass the data to next stream function if the curren stream function is nil
					response <- data
					break LOOP_READ_STREAM
				}

				// send data to downstream.
				stream, err := session.OpenUniStreamSync(context.Background())
				if err != nil {
					logger.Debug("[MergeStreamFunc] session.OpenUniStreamSync failed", "stream-fn", name)
					// pass the data to next stream function if the current stream function is nil
					response <- data
					break LOOP_READ_STREAM
				}

				_, err = stream.Write(data)
				if err == nil {
					logger.Debug("[MergeStreamFunc] YoMo-Zipper sent data to Stream Function.", "stream-fn", name)

					// close stream
					go func() {
						time.AfterFunc(time.Second, func() {
							stream.Close()
						})
					}()
					break LOOP_READ_STREAM
				}

				logger.Error("[MergeStreamFunc] YoMo-Zipper sent data to Stream Function failed.", "stream-fn", name, "err", err)
				stream.Close()
				cancel()
			}
		}
	}
}

func receiveResponseFromStreamFn(sfn GetStreamFunc, response chan []byte) {
	defer close(response)
	for {
		name, session, _ := sfn()

		if session == nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

	LOOP_ACCP_STREAM:
		for {
			stream, err := session.AcceptUniStream(context.Background())
			if stream != nil {
				logger.Debug("[MergeStreamFunc] session.AcceptUniStream", "ID", stream.StreamID())
			}

			if err != nil {
				logger.Error("[MergeStreamFunc] session.AcceptUniStream(ctx) failed", "stream-fn", name, "err", err)
				break LOOP_ACCP_STREAM
			}

			go func() {
				data, err := quic.ReadStream(stream)
				if err != nil {
					logger.Debug("[MergeStreamFunc] YoMo-Zipper received data from Stream Function failed.", "stream-fn", name, "err", err)
					return
				}

				logger.Debug("[MergeStreamFunc] YoMo-Zipper received data from Stream Function.", "stream-fn", name)
				response <- data
			}()
		}
	}
}

func sendResponseToStreamFn(ctx context.Context, next chan rxgo.Item, response chan []byte) {
	for {
		select {
		case <-ctx.Done():
			return
		case value, ok := <-response:
			if !ok {
				return
			}

			logger.Print("________ goroutine", runtime.NumGoroutine())
			if !rxgo.Of(value).SendContext(ctx, next) {
				return
			}
		}
	}
}
