package server

import (
	"context"
	"reflect"
	"time"

	"github.com/reactivex/rxgo/v2"
	"github.com/tidwall/gjson"
	"github.com/yomorun/yomo/core/quic"
	"github.com/yomorun/yomo/core/rx"
	"github.com/yomorun/yomo/internal/decoder"
	"github.com/yomorun/yomo/logger"
	"github.com/yomorun/yomo/zipper/tracing"
	"go.opentelemetry.io/otel/trace"
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

func dispatchToStreamFn(ctx context.Context, sfn GetStreamFunc, buf interface{}, next chan rxgo.Item) {
	for {
		name, session, cancel := sfn()
		data, ok := buf.([]byte)
		if !ok {
			logger.Debug("[MergeStreamFunc] the type of item.V is not a []byte", "type", reflect.TypeOf(buf))
			break
		}

		if session == nil {
			logger.Debug("[MergeStreamFunc] the session of the stream-function is nil", "stream-fn", name)
			// pass the data to next stream function if the curren stream function is nil
			rxgo.Of(data).SendContext(ctx, next)
			break
		}

		// tracing
		var traceID, spanID string
		traceIDValue := gjson.Get(string(data), `metadatas.#(name=="TraceID").value`)
		if traceIDValue.Exists() {
			traceID = traceIDValue.String()
		}
		spanIDValue := gjson.Get(string(data), `metadatas.#(name=="SpanID").value`)
		if spanIDValue.Exists() {
			spanID = spanIDValue.String()
		}

		logger.Print("Send TraceID: ", traceID, " SpanID: ", spanID)

		var span trace.Span
		if traceID != "" && spanID != "" {
			span, _ = tracing.NewRemoteTraceSpan(traceID, spanID, name, "zipper-send-to-"+name)
		}

		// send data to downstream.
		stream, err := session.OpenUniStream()
		if err != nil {
			logger.Debug("[MergeStreamFunc] session.OpenUniStream failed", "stream-fn", name)
			// pass the data to next stream function if the current stream function is nil
			rxgo.Of(data).SendContext(ctx, next)
			break
		}

		_, err = stream.Write(data)
		if err == nil {
			logger.Debug("[MergeStreamFunc] YoMo-Zipper sent data to Stream Function.", "stream-fn", name)
			// close stream
			stream.Close()
			if span != nil {
				span.End()
			}
			break
		}

		logger.Error("[MergeStreamFunc] YoMo-Zipper sent data to Stream Function failed.", "stream-fn", name, "err", err)
		stream.Close()
		cancel()
	}
}

func receiveResponseFromStreamFn(ctx context.Context, sfn GetStreamFunc, next chan rxgo.Item) {
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

				// tracing
				var traceID, spanID string
				traceIDValue := gjson.Get(string(data), `metadatas.#(name=="TraceID").value`)
				if traceIDValue.Exists() {
					traceID = traceIDValue.String()
				}
				spanIDValue := gjson.Get(string(data), `metadatas.#(name=="SpanID").value`)
				if spanIDValue.Exists() {
					spanID = spanIDValue.String()
				}

				logger.Print("Receive TraceID: ", traceID, " SpanID: ", spanID)

				var span trace.Span
				if traceID != "" && spanID != "" {
					span, _ = tracing.NewRemoteTraceSpan(traceID, spanID, name, "zipper-receive-from-"+name)
				}

				logger.Debug("[MergeStreamFunc] YoMo-Zipper received data from Stream Function.", "stream-fn", name)

				rxgo.Of(data).SendContext(ctx, next)

				if span != nil {
					span.End()
				}
			}()
		}
	}
}
