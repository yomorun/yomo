package zipper

import (
	"context"
	"reflect"
	"sync/atomic"

	"github.com/reactivex/rxgo/v2"
	"github.com/yomorun/yomo/core/quic"
	"github.com/yomorun/yomo/core/rx"
	"github.com/yomorun/yomo/internal/decoder"
	"github.com/yomorun/yomo/logger"
	// "github.com/yomorun/yomo/zipper/tracing"
)

// DispatcherWithFunc dispatches the input stream to downstreams.
func DispatcherWithFunc(ctx context.Context, sfns []GetStreamFunc, reader decoder.Reader) rx.Stream {
	stream := rx.NewFactory().FromReader(ctx, reader)

	for _, sfn := range sfns {
		stream = mergeStreamFn(ctx, stream, sfn)
	}

	return stream
}

// mergeStreamFn sends the stream data to Stream Function and receives the new stream data from it.
func mergeStreamFn(ctx context.Context, upstream rx.Stream, sfn GetStreamFunc) rx.Stream {
	f := func(ctx context.Context, next chan rxgo.Item) {
		defer close(next)
		observe := upstream.Observe()

		// send the stream to flow (zipper -> flow/sink)
		go sendDataToStreamFn(ctx, sfn, observe, next)

		// receive the response from flow  (flow/sink -> zipper)
		receiveResponseFromStreamFn(ctx, sfn, next)
	}

	return rx.CreateZipperObservable(ctx, f)
}

// sendDataToStreamFn gets the data from `upstream` and sends it to `stream-fn`.
func sendDataToStreamFn(ctx context.Context, sfn GetStreamFunc, observe <-chan rxgo.Item, next chan rxgo.Item) {
	var nextNum uint32
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

			name, funcs := sfn()
			len := len(funcs)
			// no sessions in this stream-fn.
			if len == 0 {
				continue
			}

			// only one session in this stream-fn.
			if len == 1 {
				go dispatchToStreamFn(ctx, name, funcs[0].session, funcs[0].cancel, item.V, next)
				continue
			}

			// get next session by RoundRobin when has more sessions in this stream-fn.
			n := atomic.AddUint32(&nextNum, 1)
			i := (int(n) - 1) % len
			logger.Debug("[MergeStreamFunc] dispatch data to next stream-function", "name", name, "index", i)

			go dispatchToStreamFn(ctx, name, funcs[i].session, funcs[i].cancel, item.V, next)
		}
	}
}

// dispatchToStreamFn dispatch the data to `stream-fn`.
func dispatchToStreamFn(ctx context.Context, name string, session quic.Session, cancel CancelFunc, buf interface{}, next chan rxgo.Item) {
	data, ok := buf.([]byte)
	if !ok {
		logger.Debug("[MergeStreamFunc] the type of item.V is not a []byte", "type", reflect.TypeOf(buf))
		return
	}

	if session == nil {
		logger.Error("[MergeStreamFunc] the session of the stream-function is nil", "stream-fn", name)
		// pass the data to next stream function if the curren stream function is nil
		rxgo.Of(data).SendContext(ctx, next)
		cancel()
		return
	}

	// tracing
	// span := tracing.NewSpanFromData(string(data), name, "zipper-send-to-"+name)

	// send data to downstream.
	stream, err := session.OpenUniStream()
	if err != nil {
		logger.Error("[MergeStreamFunc] session.OpenUniStream failed", "stream-fn", name)
		// pass the data to next `stream function` if the current stream function is nil
		rxgo.Of(data).SendContext(ctx, next)
		cancel()
		return
	}

	_, err = stream.Write(data)
	stream.Close()
	if err == nil {
		logger.Debug("[MergeStreamFunc] YoMo-Zipper sent data to Stream Function.", "stream-fn", name)

		// // end span in tracing
		// if span != nil {
		// 	span.End()
		// }
		return
	}

	logger.Error("[MergeStreamFunc] YoMo-Zipper sent data to Stream Function failed.", "stream-fn", name, "err", err)
	cancel()
}

// receiveResponseFromStreamFn receives the response from `stream-fn`.
func receiveResponseFromStreamFn(ctx context.Context, sfn GetStreamFunc, next chan rxgo.Item) {
	name, _ := sfn()
	ch, _ := newStreamFuncSessionCache.LoadOrStore(name, make(chan quic.Session, 5))

	for {
		select {
		case <-ctx.Done():
			return
		case session, ok := <-ch.(chan quic.Session):
			if !ok {
				return
			}

			if session == nil {
				continue
			}

			go func() {
			LOOP_ACCP_STREAM:
				for {
					stream, err := session.AcceptUniStream(context.Background())
					if err != nil {
						if err.Error() != quic.ErrConnectionClosed {
							logger.Error("[MergeStreamFunc] session.AcceptUniStream(ctx) failed", "stream-fn", name, "err", err)
						}
						break LOOP_ACCP_STREAM
					}

					go readDataFromStream(ctx, name, stream, next)
				}
			}()
		}
	}
}

// readDataFromStream reads the data from QUIC Stream.
func readDataFromStream(ctx context.Context, name string, stream quic.ReceiveStream, next chan rxgo.Item) {
	data, err := quic.ReadStream(stream)
	if err != nil {
		logger.Debug("[MergeStreamFunc] YoMo-Zipper received data from Stream Function failed.", "stream-fn", name, "err", err)
		return
	}

	logger.Debug("[MergeStreamFunc] YoMo-Zipper received data from Stream Function.", "stream-fn", name)

	// // tracing
	// span := tracing.NewSpanFromData(string(data), name, "zipper-receive-from-"+name)

	// send data to downstream.
	rxgo.Of(data).SendContext(ctx, next)

	// // end span in tracing
	// if span != nil {
	// 	span.End()
	// }
}
