package rx

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/reactivex/rxgo/v2"
	"github.com/yomorun/yomo-codec-golang/pkg/codes"
)

func FromReader(reader io.Reader) RxStream {
	next := make(chan rxgo.Item)

	go func() {
		for {
			buf := make([]byte, 3*1024)
			n, err := reader.Read(buf)

			if err != nil {
				return
			} else {
				value := buf[:n]
				next <- Of(value)
			}
		}
	}()

	return ConvertObservable(rxgo.FromChannel(next))
}

func Of(i interface{}) rxgo.Item {
	return rxgo.Item{V: i}
}

type RxStreamImpl struct {
	observable rxgo.Observable
}

func (s *RxStreamImpl) All(predicate rxgo.Predicate, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.All(predicate, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) AverageFloat32(opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.AverageFloat32(opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) AverageFloat64(opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.AverageFloat64(opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) AverageInt(opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.AverageInt(opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) AverageInt8(opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.AverageInt8(opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) AverageInt16(opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.AverageInt16(opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) AverageInt32(opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.AverageInt32(opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) AverageInt64(opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.AverageInt64(opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) BackOffRetry(backOffCfg backoff.BackOff, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.BackOffRetry(backOffCfg, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) BufferWithCount(count int, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.BufferWithCount(count, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) BufferWithTime(timespan rxgo.Duration, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.BufferWithTime(timespan, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) BufferWithTimeOrCount(timespan rxgo.Duration, count int, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.BufferWithTimeOrCount(timespan, count, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) Connect(ctx context.Context) (context.Context, rxgo.Disposable) {
	return s.observable.Connect(ctx)
}

func (s *RxStreamImpl) Contains(equal rxgo.Predicate, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.Contains(equal, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) Count(opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.Count(opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) Debounce(timespan rxgo.Duration, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.Debounce(timespan, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) DefaultIfEmpty(defaultValue interface{}, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.DefaultIfEmpty(defaultValue, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) Distinct(apply rxgo.Func, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.Distinct(apply, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) DistinctUntilChanged(apply rxgo.Func, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.DistinctUntilChanged(apply, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) DoOnCompleted(completedFunc rxgo.CompletedFunc, opts ...rxgo.Option) rxgo.Disposed {
	return s.observable.DoOnCompleted(completedFunc, opts...)
}

func (s *RxStreamImpl) DoOnError(errFunc rxgo.ErrFunc, opts ...rxgo.Option) rxgo.Disposed {
	return s.observable.DoOnError(errFunc, opts...)
}

func (s *RxStreamImpl) DoOnNext(nextFunc rxgo.NextFunc, opts ...rxgo.Option) rxgo.Disposed {
	return s.observable.DoOnNext(nextFunc, opts...)
}

func (s *RxStreamImpl) Error(opts ...rxgo.Option) error {
	return s.observable.Error(opts...)
}

func (s *RxStreamImpl) Errors(opts ...rxgo.Option) []error {
	return s.observable.Errors(opts...)
}

func (s *RxStreamImpl) Filter(apply rxgo.Predicate, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.Filter(apply, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) ElementAt(index uint, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.ElementAt(index, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) Find(find rxgo.Predicate, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.Find(find, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) First(opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.First(opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) FirstOrDefault(defaultValue interface{}, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.FirstOrDefault(defaultValue, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) FlatMap(apply rxgo.ItemToObservable, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.FlatMap(apply, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) ForEach(nextFunc rxgo.NextFunc, errFunc rxgo.ErrFunc, completedFunc rxgo.CompletedFunc, opts ...rxgo.Option) rxgo.Disposed {
	return s.observable.ForEach(nextFunc, errFunc, completedFunc, opts...)
}

func (s *RxStreamImpl) IgnoreElements(opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.IgnoreElements(opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) Join(joiner rxgo.Func2, right rxgo.Observable, timeExtractor func(interface{}) time.Time, window rxgo.Duration, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.Join(joiner, right, timeExtractor, window, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) GroupBy(length int, distribution func(rxgo.Item) int, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.GroupBy(length, distribution, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) GroupByDynamic(distribution func(rxgo.Item) string, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.GroupByDynamic(distribution, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) Last(opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.Last(opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) LastOrDefault(defaultValue interface{}, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.LastOrDefault(defaultValue, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) Map(apply rxgo.Func, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.Map(apply, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) Marshal(marshaller rxgo.Marshaller, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.Marshal(marshaller, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) Max(comparator rxgo.Comparator, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.Max(comparator, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) Min(comparator rxgo.Comparator, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.Min(comparator, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) OnErrorResumeNext(resumeSequence rxgo.ErrorToObservable, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.OnErrorResumeNext(resumeSequence, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) OnErrorReturn(resumeFunc rxgo.ErrorFunc, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.OnErrorReturn(resumeFunc, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) OnErrorReturnItem(resume interface{}, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.OnErrorReturnItem(resume, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) Reduce(apply rxgo.Func2, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.Reduce(apply, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) Repeat(count int64, frequency rxgo.Duration, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.Repeat(count, frequency, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) Retry(count int, shouldRetry func(error) bool, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.Retry(count, shouldRetry, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) Run(opts ...rxgo.Option) rxgo.Disposed {
	return s.observable.Run(opts...)
}

func (s *RxStreamImpl) Sample(iterable rxgo.Iterable, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.Sample(iterable, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) Scan(apply rxgo.Func2, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.Scan(apply, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) Send(output chan<- rxgo.Item, opts ...rxgo.Option) {
	s.observable.Send(output, opts...)
}

func (s *RxStreamImpl) SequenceEqual(iterable rxgo.Iterable, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.SequenceEqual(iterable, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) Serialize(from int, identifier func(interface{}) int, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.Serialize(from, identifier, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) Skip(nth uint, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.Skip(nth, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) SkipLast(nth uint, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.SkipLast(nth, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) SkipWhile(apply rxgo.Predicate, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.SkipWhile(apply, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) StartWith(iterable rxgo.Iterable, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.StartWith(iterable, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) SumFloat32(opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.SumFloat32(opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) SumFloat64(opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.SumFloat64(opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) SumInt64(opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.SumInt64(opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) Take(nth uint, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.Take(nth, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) TakeLast(nth uint, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.TakeLast(nth, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) TakeUntil(apply rxgo.Predicate, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.TakeUntil(apply, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) TakeWhile(apply rxgo.Predicate, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.TakeWhile(apply, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) TimeInterval(opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.TimeInterval(opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) Timestamp(opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.Timestamp(opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) ToMap(keySelector rxgo.Func, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.ToMap(keySelector, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) ToMapWithValueSelector(keySelector, valueSelector rxgo.Func, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.ToMapWithValueSelector(keySelector, valueSelector, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) ToSlice(initialCapacity int, opts ...rxgo.Option) ([]interface{}, error) {
	return s.observable.ToSlice(initialCapacity, opts...)
}

func (s *RxStreamImpl) Unmarshal(unmarshaller rxgo.Unmarshaller, factory func() interface{}, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.Unmarshal(unmarshaller, factory, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) WindowWithCount(count int, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.WindowWithCount(count, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) WindowWithTime(timespan rxgo.Duration, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.WindowWithTime(timespan, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) WindowWithTimeOrCount(timespan rxgo.Duration, count int, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.WindowWithTimeOrCount(timespan, count, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) ZipFromIterable(iterable rxgo.Iterable, zipper rxgo.Func2, opts ...rxgo.Option) RxStream {
	return &RxStreamImpl{observable: rxgo.FromChannel(s.observable.ZipFromIterable(iterable, zipper, opts...).Observe(), opts...)}
}

func (s *RxStreamImpl) Observe(opts ...rxgo.Option) <-chan rxgo.Item {
	return s.observable.Observe(opts...)
}

func (s *RxStreamImpl) DefaultIfEmptyWithTime(timespan time.Duration, defaultValue interface{}, opts ...rxgo.Option) RxStream {
	f := func(ctx context.Context, next chan rxgo.Item) {
		defer close(next)
		observe := s.Observe(opts...)

		for {
			select {
			case <-ctx.Done():
				return
			case item, ok := <-observe:
				if !ok {
					return
				}

				if !item.SendContext(ctx, next) {
					return
				}
			case <-time.After(timespan):
				if !rxgo.Of(defaultValue).SendContext(ctx, next) {
					return
				}
			}
		}
	}
	return CreateObservable(f, opts...)
}

func (s *RxStreamImpl) StdOut(opts ...rxgo.Option) RxStream {
	f := func(ctx context.Context, next chan rxgo.Item) {
		defer close(next)
		observe := s.Observe(opts...)

		for {
			select {
			case <-ctx.Done():
				return
			case item, ok := <-observe:
				if !ok {
					return
				}

				if !item.Error() {
					fmt.Println("[StdOut]: ", item.V)
				}

				if !item.SendContext(ctx, next) {
					return
				}
			}
		}
	}
	return CreateObservable(f, opts...)
}

func (s *RxStreamImpl) AuditTime(timespan time.Duration, opts ...rxgo.Option) RxStream {
	o := s.observable.BufferWithTime(rxgo.WithDuration(timespan)).Map(func(_ context.Context, i interface{}) (interface{}, error) {
		return i.([]interface{})[len(i.([]interface{}))-1], nil
	})
	return ConvertObservable(o)
}

func (s *RxStreamImpl) Y3Decoder(key string, mold interface{}, opts ...rxgo.Option) RxStream {
	codec, inform := codes.NewCodecWithInform(key)

	f := func(ctx context.Context, next chan rxgo.Item) {
		defer close(next)
		observe := s.Observe(opts...)

		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case item, ok := <-observe:
					if !ok {
						return
					}

					if item.Error() {
						return
					} else {
						codec.Decoder(item.V.([]byte))
					}
				}
			}

		}()

		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-inform:
				if !ok {
					return
				}
				value, _ := codec.Read(mold)

				if value != nil {
					if !rxgo.Of(value).SendContext(ctx, next) {
						return
					}
				} else {
					codec.Refresh(&infiniteWriter{})
				}

			}
		}
	}
	return CreateObservable(f, opts...)
}

func CreateObservable(f func(ctx context.Context, next chan rxgo.Item), opts ...rxgo.Option) RxStream {
	next := make(chan rxgo.Item)
	ctx := context.Background()
	go f(ctx, next)
	return &RxStreamImpl{observable: rxgo.FromChannel(next, opts...)}
}

func ConvertObservable(observable rxgo.Observable) RxStream {
	return &RxStreamImpl{observable: observable}
}

type infiniteWriter struct {
	io.Writer
}

func (i *infiniteWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}
