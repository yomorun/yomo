package mock

import (
	"context"
	"time"

	rx "github.com/yomorun/yomo/core/rx"
)

func channelValue(interval time.Duration, items ...interface{}) chan interface{} {
	next := make(chan interface{})
	go func() {
		for _, item := range items {
			if interval > 0 {
				time.Sleep(interval)
			}
			next <- item
		}
		close(next)
	}()
	return next
}

// Stream mocks a rx.Stream with items slice.
func Stream(items ...interface{}) rx.Stream {
	return rx.NewFactory().FromChannel(context.Background(), channelValue(0, items...))
}

// StreamWithInterval mocks a rx.Stream with items slice.
func StreamWithInterval(interval time.Duration, items ...interface{}) rx.Stream {
	return rx.NewFactory().FromChannel(context.Background(), channelValue(interval, items...))
}
