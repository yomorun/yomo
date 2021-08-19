package rx

import (
	"context"

	"github.com/reactivex/rxgo/v2"
	"github.com/yomorun/yomo/core/parser"
	"github.com/yomorun/yomo/core/quic"
	"github.com/yomorun/yomo/internal/decoder"
	"github.com/yomorun/yomo/internal/frame"
	"github.com/yomorun/yomo/logger"
)

// Factory creates the rx.Stream from several sources.
type Factory interface {
	// FromChannel creates a new Stream from a channel.
	FromChannel(ctx context.Context, channel chan interface{}) Stream

	// FromQuicStream creates a new RxStream from QUIC Stream.
	FromQuicStream(ctx context.Context, stream quic.Stream) Stream

	// FromItems creates a new Stream from items.
	FromItems(ctx context.Context, items []interface{}) Stream

	// FromItemsWithDecoder creates a new Stream from items with decoder.
	FromItemsWithDecoder(items []interface{}, opts ...decoder.Option) Stream
}

type factoryImpl struct {
}

// NewFactory creates a new Rx factory.
func NewFactory() Factory {
	return &factoryImpl{}
}

// FromChannel creates a new Stream from a channel.
func (fac *factoryImpl) FromChannel(ctx context.Context, channel chan interface{}) Stream {
	f := func(ctx context.Context, next chan rxgo.Item) {
		defer close(next)

		for {
			select {
			case <-ctx.Done():
				return
			case item, ok := <-channel:
				if !ok {
					return
				}

				switch item := item.(type) {
				default:
					Of(item).SendContext(ctx, next)
				case error:
					rxgo.Error(item).SendContext(ctx, next)
				}
			}
		}
	}
	return CreateObservable(ctx, f)
}

// FromQuicStream creates a new RxStream from QUIC Stream.
func (fac *factoryImpl) FromQuicStream(ctx context.Context, stream quic.Stream) Stream {
	f := func(ctx context.Context, next chan rxgo.Item) {
		defer close(next)

		for {
			f, err := parser.ParseFrame(stream)
			if err != nil {
				if err.Error() == quic.ErrConnectionClosed {
					logger.Error("Read the frame failed, the QUIC stream was disconnected.")
					break
				} else {
					logger.Error("Read the frame failed.", "err", err)
					continue
				}
			}

			switch f.Type() {
			case frame.TagOfDataFrame:
				dataFrame := f.(*frame.DataFrame)
				logger.Debug("Receive data frame from source.", "TransactionID", dataFrame.TransactionID())
				next <- Of(dataFrame.GetCarriage())
			default:
				logger.Debug("Only support data frame in RxStream.", "type", f.Type())
			}
		}
	}

	return CreateObservable(ctx, f)
}

// FromItems creates a new Stream from items.
func (fac *factoryImpl) FromItems(ctx context.Context, items []interface{}) Stream {
	next := make(chan rxgo.Item)
	go func() {
		for _, item := range items {
			next <- Of(item)
		}
	}()

	return ConvertObservable(ctx, rxgo.FromChannel(next))
}

// FromItemsWithDecoder creates a new Stream from items with decoder.
func (fac *factoryImpl) FromItemsWithDecoder(items []interface{}, opts ...decoder.Option) Stream {
	f := func(ctx context.Context, next chan rxgo.Item) {
		defer close(next)

		for _, item := range items {
			Of(decoder.FromItems([]interface{}{item}, opts...)).SendContext(ctx, next)
		}
	}
	return CreateObservable(decoder.GetContext(opts...), f)
}
