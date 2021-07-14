package rx

import (
	"context"
	"testing"
	"time"

	"github.com/reactivex/rxgo/v2"
	"go.uber.org/goleak"
)

// // HELPER FUNCTIONS

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

func Test_DefaultIfEmptyWithTime_Empty(t *testing.T) {
	t.Run("0 milliseconds", func(t *testing.T) {
		defer goleak.VerifyNone(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		obs := rxgo.Timer(rxgo.WithDuration(time.Millisecond))
		st := toStream(obs).DefaultIfEmptyWithTime(0, 3)
		rxgo.Assert(ctx, t, st, rxgo.IsNotEmpty())
	})

	t.Run("100 milliseconds", func(t *testing.T) {
		defer goleak.VerifyNone(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		obs := rxgo.Timer(rxgo.WithDuration(120 * time.Millisecond))
		st := toStream(obs).DefaultIfEmptyWithTime(100, 3)
		rxgo.Assert(ctx, t, st, rxgo.HasItem(3))
	})
}

func Test_DefaultIfEmptyWithTime_NotEmpty(t *testing.T) {
	defer goleak.VerifyNone(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st := toStream(rxgo.Just(1, 2, 3)()).DefaultIfEmptyWithTime(1e2, 3)
	rxgo.Assert(ctx, t, st, rxgo.HasItems(1, 2, 3))
}

func Test_StdOut_Empty(t *testing.T) {
	defer goleak.VerifyNone(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st := toStream(rxgo.Empty()).StdOut()
	rxgo.Assert(ctx, t, st, rxgo.IsEmpty())
}

func Test_StdOut_Delayed(t *testing.T) {
	defer goleak.VerifyNone(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	obs := rxgo.Timer(rxgo.WithDuration(100 * time.Millisecond))
	st := toStream(obs).StdOut()
	rxgo.Assert(ctx, t, st, rxgo.IsEmpty())
}

func Test_StdOut_NotEmpty(t *testing.T) {
	defer goleak.VerifyNone(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st := toStream(rxgo.Just(1, 2, 3)()).StdOut()
	rxgo.Assert(ctx, t, st, rxgo.HasItems(1, 2, 3))
}

func Test_AuditTime_KeepMost(t *testing.T) {
	defer goleak.VerifyNone(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st := toStream(rxgo.Just(1, 2, 3)()).AuditTime(0)
	rxgo.Assert(ctx, t, st, rxgo.IsNotEmpty())
}
