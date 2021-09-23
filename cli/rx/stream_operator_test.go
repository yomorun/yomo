package rx

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/reactivex/rxgo/v2"
	"github.com/stretchr/testify/assert"
)

// HELPER FUNCTIONS

// // Reference:
// // https://github.com/ReactiveX/RxGo/blob/master/util_test.go
// func channelValue(ctx context.Context, items ...interface{}) chan rxgo.Item {
// 	next := make(chan rxgo.Item)
// 	go func() {
// 		for _, item := range items {
// 			switch item := item.(type) {
// 			default:
// 				rxgo.Of(item).SendContext(ctx, next)
// 			case error:
// 				rxgo.Error(item).SendContext(ctx, next)
// 			}
// 		}
// 		close(next)
// 	}()
// 	return next
// }

// func newStream(ctx context.Context, items ...interface{}) Stream {
// 	return &StreamImpl{
// 		observable: rxgo.FromChannel(channelValue(ctx, items...)),
// 	}
// }

func toStream(obs rxgo.Observable) Stream {
	return &StreamImpl{observable: obs}
}

// TESTS

var testStream = toStream(rxgo.Defer([]rxgo.Producer{func(_ context.Context, ch chan<- rxgo.Item) {
	for i := 1; i <= 3; i++ {
		ch <- rxgo.Of(i)
		time.Sleep(100 * time.Millisecond)
	}
}}))

func Test_DefaultIfEmptyWithTime_Empty(t *testing.T) {
	t.Run("0 milliseconds", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		st := toStream(rxgo.Empty()).DefaultIfEmptyWithTime(0, 3)
		rxgo.Assert(ctx, t, st, rxgo.IsEmpty())
	})

	t.Run("100 milliseconds", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		obs := rxgo.Timer(rxgo.WithDuration(120 * time.Millisecond))
		st := toStream(obs).DefaultIfEmptyWithTime(100, 3)
		rxgo.Assert(ctx, t, st, rxgo.HasItem(3))
	})
}

func Test_DefaultIfEmptyWithTime_NotEmpty(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st := testStream.DefaultIfEmptyWithTime(100, 3)
	rxgo.Assert(ctx, t, st, rxgo.HasItemsNoOrder(1, 3, 2, 3, 3, 3))
}

func Test_StdOut_Empty(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st := toStream(rxgo.Empty()).StdOut()
	rxgo.Assert(ctx, t, st, rxgo.IsEmpty())
}

func Test_StdOut_NotEmpty(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st := testStream.StdOut()
	rxgo.Assert(ctx, t, st, rxgo.HasItems(1, 2, 3))
}

func Test_AuditTime(t *testing.T) {
	t.Run("0 milliseconds", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		st := testStream.AuditTime(0)
		rxgo.Assert(ctx, t, st, rxgo.HasItems(1, 2, 3))
	})

	t.Run("100 milliseconds", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		st := testStream.AuditTime(120)
		rxgo.Assert(ctx, t, st, rxgo.HasItems(2, 3))
	})

	t.Run("keep last", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		st := testStream.AuditTime(500)
		rxgo.Assert(ctx, t, st, rxgo.HasItem(3))
	})
}

type testStruct struct {
	ID   uint32 `y3:"0x11"`
	Name string `y3:"0x12"`
}

func Test_SlidingWindowWithCount(t *testing.T) {
	t.Run("window size = 1, slide size = 1, handler does nothing", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		st := testStream.SlidingWindowWithCount(1, 1, func(buf interface{}) error {
			return nil
		})
		rxgo.Assert(ctx, t, st, rxgo.HasItems(1, 2, 3))
	})

	t.Run("window size = 3, slide size = 3, handler sums elements in buf", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		st := testStream.SlidingWindowWithCount(3, 3, func(buf interface{}) error {
			slice, ok := buf.([]interface{})
			assert.Equal(t, true, ok)
			sum := 0
			for _, v := range slice {
				sum += v.(int)
			}
			assert.Equal(t, 6, sum)
			return nil
		})
		rxgo.Assert(ctx, t, st, rxgo.HasItems(1, 2, 3))
	})
}

func Test_SlidingWindowWithTime(t *testing.T) {
	t.Run("window size = 120ms, slide size = 120ms, handler does nothing", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		st := testStream.SlidingWindowWithTime(120, 120, func(buf interface{}) error {
			return nil
		})
		rxgo.Assert(ctx, t, st, rxgo.HasItems(1, 2, 3))
	})

	t.Run("window size = 360ms, slide size = 360ms, handler sums elements in buf", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		st := testStream.SlidingWindowWithTime(360, 360, func(buf interface{}) error {
			slice, ok := buf.([]interface{})
			assert.Equal(t, true, ok)
			sum := 0
			for _, v := range slice {
				sum += v.(int)
			}
			assert.Equal(t, 6, sum)
			return nil
		})
		rxgo.Assert(ctx, t, st, rxgo.HasItems(1, 2, 3))
	})
}

func Test_ContinueOnError(t *testing.T) {
	t.Run("ContinueOnError on a single operator by default", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		errFoo := errors.New("foo")
		defer cancel()
		obs := testStream.
			Map(func(_ context.Context, i interface{}) (interface{}, error) {
				if i == 2 {
					return nil, errFoo
				}
				return i, nil
			})
		rxgo.Assert(ctx, t, obs, rxgo.HasItems(1, 3), rxgo.HasError(errFoo))
	})

	t.Run("ContinueOnError on Handler by default", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		errFoo := errors.New("foo")
		defer cancel()

		handler := func(stream Stream) Stream {
			stream = stream.
				Map(func(_ context.Context, i interface{}) (interface{}, error) {
					if i == 2 {
						return nil, errFoo
					}
					return i, nil
				})
			return stream
		}

		stream := handler(testStream)
		rxgo.Assert(ctx, t, stream, rxgo.HasItems(1, 3), rxgo.HasError(errFoo))
	})
}
