package rx

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/reactivex/rxgo/v2"
	"github.com/stretchr/testify/assert"
	y3 "github.com/yomorun/y3-codec-golang"
	"github.com/yomorun/yomo/internal/decoder"
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

func Test_Subscribe_OnObserve(t *testing.T) {
	t.Run("uint32", func(t *testing.T) {
		var data uint32 = 123
		buf, _ := y3.NewCodec(0x10).Marshal(data)
		source := y3.FromStream(bytes.NewReader(buf))
		obs := source.Subscribe(0x10).OnObserve(func(v []byte) (interface{}, error) {
			i, err := y3.ToUInt32(v)
			if err != nil {
				return nil, err
			}
			assert.Equal(t, uint32(123), i)
			return i, nil
		})
		for range obs {
		} // necessary for producing human readable output
	})

	t.Run("float64", func(t *testing.T) {
		var data = 1.23
		buf, _ := y3.NewCodec(0x10).Marshal(data)
		source := y3.FromStream(bytes.NewReader(buf))
		obs := source.Subscribe(0x10).OnObserve(func(v []byte) (interface{}, error) {
			f, err := y3.ToFloat64(v)
			if err != nil {
				return nil, err
			}
			assert.Equal(t, float64(1.23), f)
			return f, nil
		})
		for range obs {
		} // necessary for producing human readable output
	})

	t.Run("string", func(t *testing.T) {
		data := "abc"
		buf, _ := y3.NewCodec(0x10).Marshal(data)
		source := y3.FromStream(bytes.NewReader(buf))
		obs := source.Subscribe(0x10).OnObserve(func(v []byte) (interface{}, error) {
			s, err := y3.ToUTF8String(v)
			if err != nil {
				return nil, err
			}
			assert.Equal(t, "abc", s)
			return s, nil
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
		} // necessary for producing human readable output
	})
}

func Test_Subscribe_MultipleKeys(t *testing.T) {
	t.Run("two", func(t *testing.T) {
		buf1, _ := y3.NewCodec(0x10).Marshal("abc")
		buf2, _ := y3.NewCodec(0x11).Marshal("def")
		source := y3.FromStream(bytes.NewReader(append(buf1, buf2...)))
		_ = source.Subscribe(0x10).OnObserve(func(v []byte) (interface{}, error) {
			str, err := y3.ToUTF8String(v)
			if err != nil {
				return nil, err
			}
			assert.Equal(t, "abc", str)
			return str, nil
		})
		_ = source.Subscribe(0x11).OnObserve(func(v []byte) (interface{}, error) {
			str, err := y3.ToUTF8String(v)
			if err != nil {
				return nil, err
			}
			assert.Equal(t, "def", str)
			return str, nil
		})
	})

	t.Run("more", func(t *testing.T) {
		buf1, _ := y3.NewCodec(0x10).Marshal("abc")
		buf2, _ := y3.NewCodec(0x11).Marshal("def")
		buf3, _ := y3.NewCodec(0x12).Marshal("uvw")
		buf4, _ := y3.NewCodec(0x13).Marshal("xyz")
		source := y3.FromStream(bytes.NewReader(append(append(append(buf1, buf2...), buf3...), buf4...)))
		_ = source.Subscribe(0x10).OnObserve(func(v []byte) (interface{}, error) {
			str, err := y3.ToUTF8String(v)
			if err != nil {
				return nil, err
			}
			assert.Equal(t, "abc", str)
			return str, nil
		})
		_ = source.Subscribe(0x11).OnObserve(func(v []byte) (interface{}, error) {
			str, err := y3.ToUTF8String(v)
			if err != nil {
				return nil, err
			}
			assert.Equal(t, "def", str)
			return str, nil
		})
		_ = source.Subscribe(0x12).OnObserve(func(v []byte) (interface{}, error) {
			str, err := y3.ToUTF8String(v)
			if err != nil {
				return nil, err
			}
			assert.Equal(t, "uvw", str)
			return str, nil
		})
		_ = source.Subscribe(0x13).OnObserve(func(v []byte) (interface{}, error) {
			str, err := y3.ToUTF8String(v)
			if err != nil {
				return nil, err
			}
			assert.Equal(t, "xyz", str)
			return str, nil
		})
	})
}

func Test_RawBytes(t *testing.T) {
	buf := &bytes.Buffer{}
	buf.Write([]byte{0, 0, 6, 0, 0, 0, 1, 2, 3})
	obs := decoder.FromStream(decoder.NewReader(buf))
	rawBytes := obs.RawBytes()
	for b := range rawBytes {
		assert.Equal(t, []byte{1, 2, 3}, b)
		break
	}
}

func Test_Encode(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		st := toStream(rxgo.Just("abc")()).Encode(0x11)
		rxgo.Assert(ctx, t, st, rxgo.HasItem([]uint8{0x81, 0x5, 0x11, 0x3, 0x61, 0x62, 0x63}))
	})

	t.Run("struct slice", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		data := []testStruct{
			{ID: 1, Name: "foo"},
			{ID: 2, Name: "bar"},
		}
		st := toStream(rxgo.Just(data)()).Encode(0x11)
		rxgo.Assert(ctx, t, st, rxgo.HasItems([]uint8{0x81, 0xa, 0x91, 0x8, 0x11, 0x1, 0x1, 0x12, 0x3, 0x66, 0x6f, 0x6f}, []uint8{0x81, 0xa, 0x91, 0x8, 0x11, 0x1, 0x2, 0x12, 0x3, 0x62, 0x61, 0x72}))
	})
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
