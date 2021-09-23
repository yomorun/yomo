package rx

import (
	"context"

	"github.com/reactivex/rxgo/v2"
)

// Factory creates the rx.Stream from several sources.
type Factory interface {
	// FromChannel creates a new Stream from a channel.
	FromChannel(ctx context.Context, channel chan interface{}) Stream

	// FromItems creates a new Stream from items.
	FromItems(ctx context.Context, items []interface{}) Stream
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
