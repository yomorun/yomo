package rx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/reactivex/rxgo/v2"
	y3 "github.com/yomorun/y3-codec-golang"
	"github.com/yomorun/yomo/internal/decoder"
	"github.com/yomorun/yomo/logger"
	"github.com/yomorun/yomo/quic"
)

// FromChannel creates a new Stream from a channel.
func FromChannel(channel chan interface{}) Stream {
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

// FromReader creates a new Stream from io.Reader.
func FromReader(reader io.Reader) Stream {
	next := make(chan rxgo.Item)

	go func() {
		defer close(next)

		fd := decoder.NewFrameDecoder(reader)
		for {
			buf, err := fd.Read(false)
			if err != nil {
				if err.Error() != quic.ErrConnectionClosed {
					logger.Error("Receive frame from source failed.", "err", err)
				}
				break
			} else {
				logger.Debug("Receive frame from source.", "frame", logger.BytesString(buf))
				next <- Of(buf)
			}
		}
	}()

	return ConvertObservable(rxgo.FromChannel(next, rxgo.WithErrorStrategy(rxgo.ContinueOnError)))
}

// FromReaderWithDecoder creates a Stream with decoder.
func FromReaderWithDecoder(readers chan io.Reader) Stream {
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
	return CreateObservable(f, rxgo.WithPublishStrategy())
}

func FromReaderWithFunc(f func() io.Reader) Stream {
	next := make(chan rxgo.Item)

	go func() {
		for {
			reader := f()

			if reader == nil {
				time.Sleep(time.Second)
			} else {
				fd := decoder.NewFrameDecoder(reader)
				for {
					buf, err := fd.Read(false)
					if err != nil {
						break
					} else {
						next <- Of(buf)
					}
				}
			}
		}
	}()

	return ConvertObservable(rxgo.FromChannel(next, rxgo.WithErrorStrategy(rxgo.ContinueOnError)))
}

func Of(i interface{}) rxgo.Item {
	return rxgo.Item{V: i}
}

// StreamImpl is the implementation of Stream.
type StreamImpl struct {
	observable rxgo.Observable
}

// appendContinueOnError appends the "ContinueOnError" to options
func appendContinueOnError(opts ...rxgo.Option) []rxgo.Option {
	return append(opts, rxgo.WithErrorStrategy(rxgo.ContinueOnError))
}

func (s *StreamImpl) All(predicate rxgo.Predicate, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.All(predicate, opts...).Observe(), opts...)}
}

func (s *StreamImpl) AverageFloat32(opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.AverageFloat32(opts...).Observe(), opts...)}
}

func (s *StreamImpl) AverageFloat64(opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.AverageFloat64(opts...).Observe(), opts...)}
}

func (s *StreamImpl) AverageInt(opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.AverageInt(opts...).Observe(), opts...)}
}

func (s *StreamImpl) AverageInt8(opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.AverageInt8(opts...).Observe(), opts...)}
}

func (s *StreamImpl) AverageInt16(opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.AverageInt16(opts...).Observe(), opts...)}
}

func (s *StreamImpl) AverageInt32(opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.AverageInt32(opts...).Observe(), opts...)}
}

func (s *StreamImpl) AverageInt64(opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.AverageInt64(opts...).Observe(), opts...)}
}

func (s *StreamImpl) BackOffRetry(backOffCfg backoff.BackOff, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.BackOffRetry(backOffCfg, opts...).Observe(), opts...)}
}

func (s *StreamImpl) BufferWithCount(count int, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.BufferWithCount(count, opts...).Observe(), opts...)}
}

func getRxDuration(milliseconds uint32) rxgo.Duration {
	return rxgo.WithDuration(time.Duration(milliseconds) * time.Millisecond)
}

func (s *StreamImpl) BufferWithTime(milliseconds uint32, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.BufferWithTime(getRxDuration(milliseconds), opts...).Observe(), opts...)}
}

func (s *StreamImpl) BufferWithTimeOrCount(milliseconds uint32, count int, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.BufferWithTimeOrCount(getRxDuration(milliseconds), count, opts...).Observe(), opts...)}
}

func (s *StreamImpl) Connect(ctx context.Context) (context.Context, rxgo.Disposable) {
	return s.observable.Connect(ctx)
}

func (s *StreamImpl) Contains(equal rxgo.Predicate, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.Contains(equal, opts...).Observe(), opts...)}
}

func (s *StreamImpl) Count(opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.Count(opts...).Observe(), opts...)}
}

func (s *StreamImpl) Debounce(milliseconds uint32, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.Debounce(getRxDuration(milliseconds), opts...).Observe(), opts...)}
}

func (s *StreamImpl) DefaultIfEmpty(defaultValue interface{}, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.DefaultIfEmpty(defaultValue, opts...).Observe(), opts...)}
}

func (s *StreamImpl) Distinct(apply rxgo.Func, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.Distinct(apply, opts...).Observe(), opts...)}
}

func (s *StreamImpl) DistinctUntilChanged(apply rxgo.Func, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.DistinctUntilChanged(apply, opts...).Observe(), opts...)}
}

func (s *StreamImpl) DoOnCompleted(completedFunc rxgo.CompletedFunc, opts ...rxgo.Option) rxgo.Disposed {
	opts = appendContinueOnError(opts...)
	return s.observable.DoOnCompleted(completedFunc, opts...)
}

func (s *StreamImpl) DoOnError(errFunc rxgo.ErrFunc, opts ...rxgo.Option) rxgo.Disposed {
	opts = appendContinueOnError(opts...)
	return s.observable.DoOnError(errFunc, opts...)
}

func (s *StreamImpl) DoOnNext(nextFunc rxgo.NextFunc, opts ...rxgo.Option) rxgo.Disposed {
	opts = appendContinueOnError(opts...)
	return s.observable.DoOnNext(nextFunc, opts...)
}

func (s *StreamImpl) Error(opts ...rxgo.Option) error {
	opts = appendContinueOnError(opts...)
	return s.observable.Error(opts...)
}

func (s *StreamImpl) Errors(opts ...rxgo.Option) []error {
	opts = appendContinueOnError(opts...)
	return s.observable.Errors(opts...)
}

func (s *StreamImpl) Filter(apply rxgo.Predicate, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.Filter(apply, opts...).Observe(), opts...)}
}

func (s *StreamImpl) ElementAt(index uint, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.ElementAt(index, opts...).Observe(), opts...)}
}

func (s *StreamImpl) Find(find rxgo.Predicate, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.Find(find, opts...).Observe(), opts...)}
}

func (s *StreamImpl) First(opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.First(opts...).Observe(), opts...)}
}

func (s *StreamImpl) FirstOrDefault(defaultValue interface{}, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.FirstOrDefault(defaultValue, opts...).Observe(), opts...)}
}

func (s *StreamImpl) FlatMap(apply rxgo.ItemToObservable, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.FlatMap(apply, opts...).Observe(), opts...)}
}

func (s *StreamImpl) ForEach(nextFunc rxgo.NextFunc, errFunc rxgo.ErrFunc, completedFunc rxgo.CompletedFunc, opts ...rxgo.Option) rxgo.Disposed {
	opts = appendContinueOnError(opts...)
	return s.observable.ForEach(nextFunc, errFunc, completedFunc, opts...)
}

func (s *StreamImpl) IgnoreElements(opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.IgnoreElements(opts...).Observe(), opts...)}
}

func (s *StreamImpl) Join(joiner rxgo.Func2, right rxgo.Observable, timeExtractor func(interface{}) time.Time, windowInMS uint32, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.Join(joiner, right, timeExtractor, getRxDuration(windowInMS), opts...).Observe(), opts...)}
}

func (s *StreamImpl) GroupBy(length int, distribution func(rxgo.Item) int, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.GroupBy(length, distribution, opts...).Observe(), opts...)}
}

func (s *StreamImpl) GroupByDynamic(distribution func(rxgo.Item) string, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.GroupByDynamic(distribution, opts...).Observe(), opts...)}
}

func (s *StreamImpl) Last(opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.Last(opts...).Observe(), opts...)}
}

func (s *StreamImpl) LastOrDefault(defaultValue interface{}, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.LastOrDefault(defaultValue, opts...).Observe(), opts...)}
}

func (s *StreamImpl) Map(apply rxgo.Func, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.Map(apply, opts...).Observe(), opts...)}
}

// Marshal transforms the items emitted by an Observable by applying a marshalling to each item.
func (s *StreamImpl) Marshal(marshaller decoder.Marshaller, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)

	return s.Map(func(_ context.Context, i interface{}) (interface{}, error) {
		return marshaller(i)
	}, opts...)
}

// Unmarshal transforms the items emitted by an Observable by applying an unmarshalling to each item.
func (s *StreamImpl) Unmarshal(unmarshaller decoder.Unmarshaller, factory func() interface{}, opts ...rxgo.Option) Stream {
	f := func(ctx context.Context, next chan rxgo.Item) {
		defer close(next)
		observe := s.Observe()
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
				}
				go func() {
					onObserve := (item.V).(decoder.Observable).Unmarshal(unmarshaller, factory)

					for {
						select {
						case <-ctx.Done():
							return
						case item, ok := <-onObserve:
							if !ok {
								return
							}
							if !Of(item).SendContext(ctx, next) {
								return
							}
						}
					}
				}()
			}
		}
	}
	return CreateObservable(f)
}

func (s *StreamImpl) Max(comparator rxgo.Comparator, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.Max(comparator, opts...).Observe(), opts...)}
}

func (s *StreamImpl) Min(comparator rxgo.Comparator, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.Min(comparator, opts...).Observe(), opts...)}
}

func (s *StreamImpl) OnErrorResumeNext(resumeSequence rxgo.ErrorToObservable, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.OnErrorResumeNext(resumeSequence, opts...).Observe(), opts...)}
}

func (s *StreamImpl) OnErrorReturn(resumeFunc rxgo.ErrorFunc, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.OnErrorReturn(resumeFunc, opts...).Observe(), opts...)}
}

func (s *StreamImpl) OnErrorReturnItem(resume interface{}, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.OnErrorReturnItem(resume, opts...).Observe(), opts...)}
}

func (s *StreamImpl) Reduce(apply rxgo.Func2, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.Reduce(apply, opts...).Observe(), opts...)}
}

func (s *StreamImpl) Repeat(count int64, milliseconds uint32, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.Repeat(count, getRxDuration(milliseconds), opts...).Observe(), opts...)}
}

func (s *StreamImpl) Retry(count int, shouldRetry func(error) bool, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.Retry(count, shouldRetry, opts...).Observe(), opts...)}
}

func (s *StreamImpl) Run(opts ...rxgo.Option) rxgo.Disposed {
	opts = appendContinueOnError(opts...)
	return s.observable.Run(opts...)
}

func (s *StreamImpl) Sample(iterable rxgo.Iterable, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.Sample(iterable, opts...).Observe(), opts...)}
}

func (s *StreamImpl) Scan(apply rxgo.Func2, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.Scan(apply, opts...).Observe(), opts...)}
}

func (s *StreamImpl) Send(output chan<- rxgo.Item, opts ...rxgo.Option) {
	opts = appendContinueOnError(opts...)
	s.observable.Send(output, opts...)
}

func (s *StreamImpl) SequenceEqual(iterable rxgo.Iterable, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.SequenceEqual(iterable, opts...).Observe(), opts...)}
}

func (s *StreamImpl) Serialize(from int, identifier func(interface{}) int, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.Serialize(from, identifier, opts...).Observe(), opts...)}
}

func (s *StreamImpl) Skip(nth uint, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.Skip(nth, opts...).Observe(), opts...)}
}

func (s *StreamImpl) SkipLast(nth uint, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.SkipLast(nth, opts...).Observe(), opts...)}
}

func (s *StreamImpl) SkipWhile(apply rxgo.Predicate, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.SkipWhile(apply, opts...).Observe(), opts...)}
}

func (s *StreamImpl) StartWith(iterable rxgo.Iterable, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.StartWith(iterable, opts...).Observe(), opts...)}
}

func (s *StreamImpl) SumFloat32(opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.SumFloat32(opts...).Observe(), opts...)}
}

func (s *StreamImpl) SumFloat64(opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.SumFloat64(opts...).Observe(), opts...)}
}

func (s *StreamImpl) SumInt64(opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.SumInt64(opts...).Observe(), opts...)}
}

func (s *StreamImpl) Take(nth uint, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.Take(nth, opts...).Observe(), opts...)}
}

func (s *StreamImpl) TakeLast(nth uint, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.TakeLast(nth, opts...).Observe(), opts...)}
}

func (s *StreamImpl) TakeUntil(apply rxgo.Predicate, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.TakeUntil(apply, opts...).Observe(), opts...)}
}

func (s *StreamImpl) TakeWhile(apply rxgo.Predicate, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.TakeWhile(apply, opts...).Observe(), opts...)}
}

func (s *StreamImpl) TimeInterval(opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.TimeInterval(opts...).Observe(), opts...)}
}

func (s *StreamImpl) Timestamp(opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.Timestamp(opts...).Observe(), opts...)}
}

func (s *StreamImpl) ToMap(keySelector rxgo.Func, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.ToMap(keySelector, opts...).Observe(), opts...)}
}

func (s *StreamImpl) ToMapWithValueSelector(keySelector, valueSelector rxgo.Func, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.ToMapWithValueSelector(keySelector, valueSelector, opts...).Observe(), opts...)}
}

func (s *StreamImpl) ToSlice(initialCapacity int, opts ...rxgo.Option) ([]interface{}, error) {
	opts = appendContinueOnError(opts...)
	return s.observable.ToSlice(initialCapacity, opts...)
}

func (s *StreamImpl) WindowWithCount(count int, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.WindowWithCount(count, opts...).Observe(), opts...)}
}

func (s *StreamImpl) WindowWithTime(milliseconds uint32, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.WindowWithTime(getRxDuration(milliseconds), opts...).Observe(), opts...)}
}

func (s *StreamImpl) WindowWithTimeOrCount(milliseconds uint32, count int, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.WindowWithTimeOrCount(getRxDuration(milliseconds), count, opts...).Observe(), opts...)}
}

func (s *StreamImpl) ZipFromIterable(iterable rxgo.Iterable, zipper rxgo.Func2, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(s.observable.ZipFromIterable(iterable, zipper, opts...).Observe(), opts...)}
}

func (s *StreamImpl) Observe(opts ...rxgo.Option) <-chan rxgo.Item {
	opts = appendContinueOnError(opts...)
	return s.observable.Observe(opts...)
}

func (s *StreamImpl) DefaultIfEmptyWithTime(milliseconds uint32, defaultValue interface{}, opts ...rxgo.Option) Stream {
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
			case <-time.After(time.Duration(milliseconds) * time.Millisecond):
				if !rxgo.Of(defaultValue).SendContext(ctx, next) {
					return
				}
			}
		}
	}
	return CreateObservable(f, opts...)
}

func (s *StreamImpl) StdOut(opts ...rxgo.Option) Stream {
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

func (s *StreamImpl) AuditTime(milliseconds uint32, opts ...rxgo.Option) Stream {
	opts = appendContinueOnError(opts...)
	o := s.observable.BufferWithTime(getRxDuration(milliseconds), opts...).Map(func(_ context.Context, i interface{}) (interface{}, error) {
		return i.([]interface{})[len(i.([]interface{}))-1], nil
	}, opts...)
	return ConvertObservable(o)
}

// Subscribe a specified key in stream and gets the data when the key is observed by Y3 Codec.
func (s *StreamImpl) Subscribe(key byte) Stream {

	f := func(ctx context.Context, next chan rxgo.Item) {
		defer close(next)
		observe := s.Observe()
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
				}
				y3stream, ok := (item.V).(decoder.Observable)
				if !ok {
					logger.Error("[Subscribe] the type of item.V is not `decoder.Observable`")
					return
				}

				if !Of(y3stream.Subscribe(key)).SendContext(ctx, next) {
					return
				}
			}
		}
	}
	return CreateObservable(f)
}

// RawBytes get the raw bytes in Stream which receives from yomo-server.
func (s *StreamImpl) RawBytes() Stream {
	f := func(ctx context.Context, next chan rxgo.Item) {
		defer close(next)
		observe := s.Observe()
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
				}
				y3stream, ok := (item.V).(decoder.Observable)
				if !ok {
					logger.Error("[RawBytes] the type of item.V is not `decoder.Observable`")
					return
				}

				bufCh := y3stream.RawBytes()
				go func() {
					for buf := range bufCh {
						logger.Debug("[RawBytes] get the raw bytes from yomo-server.", "buf", logger.BytesString(buf))
						Of(buf).SendContext(ctx, next)
					}
				}()
			}
		}
	}
	return CreateObservable(f)
}

// ZipMultiObservers subscribes multi Y3 observers, zips the values into a slice and calls the zipper callback when all keys are observed.
func (s *StreamImpl) ZipMultiObservers(observers []KeyObserveFunc, zipper func(items []interface{}) (interface{}, error)) Stream {
	count := len(observers)
	if count < 2 {
		return s.thrown(errors.New("[ZipMultiObservers] the number of observers must be >= 2"))
	}

	// the function to zip the values into a slice
	var zipObserveFunc = func(_ context.Context, a interface{}, b interface{}) (interface{}, error) {
		items, ok := a.([]interface{})
		if !ok {
			return []interface{}{a, b}, nil
		}

		items = append(items, b)
		return items, nil
	}

	// the function of the `ZipMultiObservers` operator
	f := func(ctx context.Context, next chan rxgo.Item) {
		defer close(next)

		// prepare slices and maps
		keys := make([]byte, count)
		keyObserveMap := make(map[byte]decoder.OnObserveFunc, count)
		keyChans := make(map[byte]chan rxgo.Item, count)
		keyObservables := make([]rxgo.Observable, count)
		for i, item := range observers {
			keys[i] = item.Key
			keyObserveMap[item.Key] = item.OnObserve
			ch := make(chan rxgo.Item)
			keyChans[item.Key] = ch
			keyObservables[i] = rxgo.FromChannel(ch)
		}

		// zip all observables
		zipObservable := keyObservables[0]
		for i := 1; i < count; i++ {
			zipObservable = zipObservable.ZipFromIterable(keyObservables[i], zipObserveFunc)
		}

		observe := s.Observe()
		go func() {
			defer func() {
				for _, ch := range keyChans {
					close(ch)
				}
			}()

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
					}

					y3stream := (item.V).(decoder.Observable)
					// subscribe multi keys
					y3Observable := y3stream.MultiSubscribe(keys...)
					go func() {
						// get the value when the key is observed
						kvCh := y3Observable.OnMultiObserve(keyObserveMap)
						for {
							select {
							case <-ctx.Done():
								return
							case kv, ok := <-kvCh:
								if !ok {
									return
								}

								ch := keyChans[kv.Key]
								if ch != nil {
									ch <- rxgo.Item{V: kv.Value}
								} else {
									ch <- rxgo.Item{E: fmt.Errorf("[ZipMultiObservers] ch is not found for key %v", kv.Key)}
								}
							}
						}
					}()
				}
			}
		}()

		for {
			// observe the value from zipObservable
			for item := range zipObservable.Observe(rxgo.WithErrorStrategy(rxgo.ContinueOnError)) {
				if item.Error() {
					logger.Error("[ZipMultiObservers] observe the value failed.", "err", item.E)
					continue
				}

				items, ok := item.V.([]interface{})
				if !ok {
					logger.Error("[ZipMultiObservers] item.V is not a slice")
					continue
				}

				// call the zipper callback
				v, err := zipper(items)
				if err != nil {
					logger.Error("[ZipMultiObservers] zipper func returns an err.", "err", err)
					continue
				}

				if !Of(v).SendContext(ctx, next) {
					return
				}
			}
		}
	}
	return CreateObservable(f)
}

// OnObserve calls the function to process the observed data.
func (s *StreamImpl) OnObserve(function func(v []byte) (interface{}, error)) Stream {

	f := func(ctx context.Context, next chan rxgo.Item) {
		defer close(next)
		observe := s.Observe()
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
				}
				go func() {
					onObserve := (item.V).(decoder.Observable).OnObserve(function)

					for {
						select {
						case <-ctx.Done():
							return
						case item, ok := <-onObserve:
							if !ok {
								return
							}
							logger.Debug("[OnObserve] Get data after OnObserve.", "data", item)
							if !Of(item).SendContext(ctx, next) {
								return
							}
						}
					}
				}()
			}
		}
	}
	return CreateObservable(f)
}

// Encode the data with a specified key by Y3 Codec and append it to stream.
func (s *StreamImpl) Encode(key byte, opts ...rxgo.Option) Stream {
	y3codec := y3.NewCodec(key)

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

				if item.Error() {
					return
				}

				buf, err := y3codec.Marshal(item.V)

				if err != nil {
					logger.Debug("[Encode Operator] encodes data failed via Y3 Codec.", "key", key, "data", item.V, "err", err)
					continue
				}

				if !Of(buf).SendContext(ctx, next) {
					return
				}
			}
		}
	}
	return CreateObservable(f, opts...)
}

// SlidingWindowWithCount buffers the data in the specified sliding window size, the buffered data can be processed in the handler func.
// It returns the orginal data to Stream, not the buffered slice.
func (s *StreamImpl) SlidingWindowWithCount(windowSize int, slideSize int, handler Handler, opts ...rxgo.Option) Stream {
	if windowSize <= 0 {
		return s.thrown(errors.New("windowSize must be positive"))
	}
	if slideSize <= 0 {
		return s.thrown(errors.New("slideSize must be positive"))
	}

	f := func(ctx context.Context, next chan rxgo.Item) {
		defer close(next)
		observe := s.Observe()

		windowCount := 0
		currentSlideCount := 0
		buf := make([]interface{}, windowSize)
		firstTimeSend := true
		mutex := sync.Mutex{}

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
				}

				mutex.Lock()
				if windowCount < windowSize {
					buf[windowCount] = item.V
					windowCount++
				}

				if windowCount == windowSize {
					// start sliding
					currentSlideCount++

					// append slide item to buffer
					if !firstTimeSend {
						buf = append(buf[1:windowSize], item.V)
					}

					// reach slide size
					if currentSlideCount%slideSize == 0 {
						err := handler(buf)
						firstTimeSend = false
						if err != nil {
							rxgo.Error(err).SendContext(ctx, next)
							return
						}
					}
				}
				mutex.Unlock()
				// immediately send the original item to downstream
				Of(item.V).SendContext(ctx, next)
			}
		}
	}
	return CreateObservable(f, opts...)
}

// SlidingWindowWithTime buffers the data in the specified sliding window time, the buffered data can be processed in the handler func.
// It returns the orginal data to Stream, not the buffered slice.
func (s *StreamImpl) SlidingWindowWithTime(windowTimeInMS uint32, slideTimeInMS uint32, handler Handler, opts ...rxgo.Option) Stream {
	f := func(ctx context.Context, next chan rxgo.Item) {
		observe := s.Observe()
		buf := make([]slidingWithTimeItem, 0)
		stop := make(chan struct{})
		firstTimeSend := true
		mutex := sync.Mutex{}

		checkBuffer := func() {
			mutex.Lock()
			// filter items by time
			updatedBuf := make([]slidingWithTimeItem, 0)
			availableItems := make([]interface{}, 0)
			t := time.Now().Add(-time.Duration(windowTimeInMS) * time.Millisecond)
			for _, item := range buf {
				if item.timestamp.After(t) || item.timestamp.Equal(t) {
					updatedBuf = append(updatedBuf, item)
					availableItems = append(availableItems, item.data)
				}
			}
			buf = updatedBuf

			// apply and send items
			if len(availableItems) != 0 {
				err := handler(availableItems)
				if err != nil {
					rxgo.Error(err).SendContext(ctx, next)
					return
				}
			}
			firstTimeSend = false
			mutex.Unlock()
		}

		go func() {
			defer close(next)
			for {
				select {
				case <-stop:
					checkBuffer()
					return
				case <-ctx.Done():
					return
				case <-time.After(time.Duration(windowTimeInMS) * time.Millisecond):
					if firstTimeSend {
						checkBuffer()
					}
				case <-time.After(time.Duration(slideTimeInMS) * time.Millisecond):
					checkBuffer()
				}
			}
		}()

		for {
			select {
			case <-ctx.Done():
				close(stop)
				return
			case item, ok := <-observe:
				if !ok {
					close(stop)
					return
				}
				if item.Error() {
					item.SendContext(ctx, next)
					close(stop)
					return
				} else {
					mutex.Lock()
					// buffer data
					buf = append(buf, slidingWithTimeItem{
						timestamp: time.Now(),
						data:      item.V,
					})
					mutex.Unlock()
				}
				// immediately send the original item to downstream
				Of(item.V).SendContext(ctx, next)
			}
		}
	}
	return CreateObservable(f, opts...)
}

type slidingWithTimeItem struct {
	timestamp time.Time
	data      interface{}
}

// Handler defines a function that handle the input value.
type Handler func(interface{}) error

func (s *StreamImpl) thrown(err error) Stream {
	next := make(chan rxgo.Item, 1)
	next <- rxgo.Error(err)
	defer close(next)
	return &StreamImpl{observable: rxgo.FromChannel(next)}
}

func CreateObservable(f func(ctx context.Context, next chan rxgo.Item), opts ...rxgo.Option) Stream {
	next := make(chan rxgo.Item)
	ctx := context.Background()
	go f(ctx, next)
	opts = appendContinueOnError(opts...)
	return &StreamImpl{observable: rxgo.FromChannel(next, opts...)}
}

func ConvertObservable(observable rxgo.Observable) Stream {
	return &StreamImpl{observable: observable}
}

type infiniteWriter struct {
	io.Writer
}

func (i *infiniteWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}
