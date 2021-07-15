package rx

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/reactivex/rxgo/v2"
	"github.com/stretchr/testify/assert"
	y3 "github.com/yomorun/y3-codec-golang"
	"go.uber.org/goleak"
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
		defer goleak.VerifyNone(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		st := toStream(rxgo.Empty()).DefaultIfEmptyWithTime(0, 3)
		rxgo.Assert(ctx, t, st, rxgo.IsEmpty())
	})

	t.Run("100 milliseconds", func(t *testing.T) {
		defer goleak.VerifyNone(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		obs := rxgo.Timer(rxgo.WithDuration(100 * time.Millisecond))
		st := toStream(obs).DefaultIfEmptyWithTime(100, 3)
		rxgo.Assert(ctx, t, st, rxgo.HasItem(3))
	})
}

func Test_DefaultIfEmptyWithTime_NotEmpty(t *testing.T) {
	defer goleak.VerifyNone(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st := testStream.DefaultIfEmptyWithTime(100, 3)
	rxgo.Assert(ctx, t, st, rxgo.HasItems(1, 3, 2, 3, 3, 3))
}

func Test_StdOut_Empty(t *testing.T) {
	defer goleak.VerifyNone(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st := toStream(rxgo.Empty()).StdOut()
	rxgo.Assert(ctx, t, st, rxgo.IsEmpty())
}

func Test_StdOut_NotEmpty(t *testing.T) {
	defer goleak.VerifyNone(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st := testStream.StdOut()
	rxgo.Assert(ctx, t, st, rxgo.HasItems(1, 2, 3))
}

func Test_AuditTime(t *testing.T) {
	t.Run("0 milliseconds", func(t *testing.T) {
		defer goleak.VerifyNone(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		st := testStream.AuditTime(0)
		rxgo.Assert(ctx, t, st, rxgo.HasItems(1, 2, 3))
	})

	t.Run("100 milliseconds", func(t *testing.T) {
		defer goleak.VerifyNone(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		st := testStream.AuditTime(100)
		rxgo.Assert(ctx, t, st, rxgo.HasItems(2, 3))
	})

	t.Run("keep last", func(t *testing.T) {
		defer goleak.VerifyNone(t)
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

func Test_Subscribe_OnObserve(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		data := "abc"
		buf, _ := y3.NewCodec(0x10).Marshal(data)
		source := y3.FromStream(bytes.NewReader(buf))
		obs := source.Subscribe(0x10).OnObserve(func(v []byte) (interface{}, error) {
			str, err := y3.ToUTF8String(v)
			if err != nil {
				return nil, err
			}
			assert.Equal(t, "abc", str)
			return str, nil
		})
		for range obs {
		} // necessary for producing human readable output
	})

	t.Run("struct slice", func(t *testing.T) {
		data := []testStruct{
			{ID: 1, Name: "foo"},
			{ID: 2, Name: "bar"},
		}
		buf, _ := y3.NewCodec(0x10).Marshal(data)
		source := y3.FromStream(bytes.NewReader(buf))
		obs := source.Subscribe(0x10).OnObserve(func(v []byte) (interface{}, error) {
			var s []testStruct
			err := y3.ToObject(v, &s)
			if err != nil {
				return nil, err
			}
			assert.Equal(t, []testStruct{{ID: 1, Name: "foo"}, {ID: 2, Name: "bar"}}, s)
			return s, nil
		})
		for range obs {
		}
	})
}

func Test_RawBytes(t *testing.T) {
	// TODO
}

func Test_ZipMultiObservers(t *testing.T) {
	// TODO
}

func Test_Encode(t *testing.T) {
	// TODO
}

func Test_SlidingWindowWithCount(t *testing.T) {
	// TODO
}

func Test_SlidingWindowWithTime(t *testing.T) {
	// TODO
}
