package rx

import "time"

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

// MockStream mocks a rx.Stream with items slice.
func MockStream(items ...interface{}) Stream {
	return FromChannel(channelValue(0, items...))
}

// MockStream mocks a rx.Stream with items slice.
func MockStreamWithInterval(interval time.Duration, items ...interface{}) Stream {
	return FromChannel(channelValue(interval, items...))
}
