package server

import (
	"context"
	"reflect"
	"time"

	"github.com/reactivex/rxgo/v2"
	"github.com/yomorun/yomo/core/quic"
	"github.com/yomorun/yomo/core/rx"
	"github.com/yomorun/yomo/logger"
)

// DispatcherWithFunc dispatches the input stream to downstreams.
func DispatcherWithFunc(ctx context.Context, sfns []GetStreamFunc, stream quic.Stream) rx.Stream {
	rxStream := rx.NewFactory().FromQuicStream(ctx, stream)

	for _, sfn := range sfns {
		rxStream = mergeStreamFn(ctx, rxStream, sfn)
	}

	return rxStream
}

// mergeStreamFn sends the stream data to Stream Function and receives the new stream data from it. (in Rx way)
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

			go dispatchToStreamFn(ctx, sfn, item.V, next)
		}
	}
}

// dispatchToStreamFn dispatch the data to `stream-fn`.
func dispatchToStreamFn(ctx context.Context, sfn GetStreamFunc, buf interface{}, next chan rxgo.Item) {
	name, session, cancel := sfn()
	data, ok := buf.([]byte)
	if !ok {
		logger.Debug("[MergeStreamFunc] the type of item.V is not a []byte", "type", reflect.TypeOf(buf))
		return
	}

	if session == nil {
		logger.Debug("[MergeStreamFunc] the session of the stream-function is nil", "stream-fn", name)
		// pass the data to next stream function if the curren stream function is nil
		rxgo.Of(data).SendContext(ctx, next)
		return
	}

	// // tracing
	// span := tracing.NewSpanFromData(string(data), name, "zipper-send-to-"+name)

	// create a new QUIC stream.
	stream, err := session.OpenUniStream()
	if err != nil {
		logger.Debug("[MergeStreamFunc] session.OpenUniStream failed", "stream-fn", name)
		// pass the data to next `stream function` if the current stream function is nil
		rxgo.Of(data).SendContext(ctx, next)
		return
	}

	// send data to downstream. (no frame at this moment)
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
	for {
		name, session, _ := sfn()

		if session == nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

	LOOP_ACCP_STREAM:
		for {
			// accept stream from `stream-fn`.
			stream, err := session.AcceptUniStream(context.Background())
			if err != nil {
				if err.Error() != quic.ErrConnectionClosed {
					logger.Error("[MergeStreamFunc] session.AcceptUniStream(ctx) failed", "stream-fn", name, "err", err)
				}
				break LOOP_ACCP_STREAM
			}

			go readDataFromStream(ctx, name, stream, next)
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

	// send data to "next" stream function.
	rxgo.Of(data).SendContext(ctx, next)

	// // end span in tracing
	// if span != nil {
	// 	span.End()
	// }
}
