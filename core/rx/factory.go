package rx

import (
	"context"

	"github.com/reactivex/rxgo/v2"
	"github.com/yomorun/yomo/internal/decoder"
	"github.com/yomorun/yomo/logger"
)

// Factory creates the rx.Stream from several sources.
type Factory interface {
	// FromChannel creates a new Stream from a channel.
	FromChannel(channel chan interface{}) Stream

	// FromReader creates a new Stream from io.Reader.
	FromReader(reader decoder.Reader) Stream

	// FromReaderWithDecoder creates a new Stream with decoder.
	FromReaderWithDecoder(readers chan decoder.Reader) Stream
}

type factoryImpl struct {
}

func NewFactory() Factory {
	return &factoryImpl{}
}

// FromChannel creates a new Stream from a channel.
func (fac *factoryImpl) FromChannel(channel chan interface{}) Stream {
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
	return CreateObservable(f)
}

// FromReader creates a new Stream from decoder.Reader.
func (fac *factoryImpl) FromReader(reader decoder.Reader) Stream {
	next := make(chan rxgo.Item)

	go func() {
		defer close(next)

		frameChan := reader.Read()
		for frame := range frameChan {
			logger.Debug("Receive frame from source.", "frame", logger.BytesString(frame.Bytes()))
			next <- Of(frame)
		}
	}()

	return ConvertObservable(rxgo.FromChannel(next, rxgo.WithErrorStrategy(rxgo.ContinueOnError)))
}

// FromReaderWithDecoder creates a Stream with decoder.
func (fac *factoryImpl) FromReaderWithDecoder(readers chan decoder.Reader) Stream {
	f := func(ctx context.Context, next chan rxgo.Item) {
		defer close(next)

		for {
			select {
			case <-ctx.Done():
				return
			case item, ok := <-readers:
				if !ok {
					return
				}

				Of(decoder.FromStream(item)).SendContext(ctx, next)
			}
		}
	}
	return CreateObservable(f)
}
