package rx

import (
	"context"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/reactivex/rxgo/v2"
	"github.com/yomorun/yomo/internal/decoder"
)

// Stream is the interface for RxStream.
type Stream interface {
	rxgo.Iterable

	// Subscribe a specified key in stream and gets the data when the key is observed by Y3 Codec.
	Subscribe(key byte) Stream

	// OnObserve calls the function to process the observed data.
	OnObserve(function func(v []byte) (interface{}, error)) Stream

	// Encode the data with a specified key by Y3 Codec and append it to stream.
	Encode(key byte, opts ...rxgo.Option) Stream

	// RawBytes get the raw bytes in Stream which receives from yomo-server.
	RawBytes() Stream

	StdOut(opts ...rxgo.Option) Stream
	AuditTime(milliseconds uint32, opts ...rxgo.Option) Stream
	DefaultIfEmptyWithTime(milliseconds uint32, defaultValue interface{}, opts ...rxgo.Option) Stream
	All(predicate rxgo.Predicate, opts ...rxgo.Option) Stream
	AverageFloat32(opts ...rxgo.Option) Stream
	AverageFloat64(opts ...rxgo.Option) Stream
	AverageInt(opts ...rxgo.Option) Stream
	AverageInt8(opts ...rxgo.Option) Stream
	AverageInt16(opts ...rxgo.Option) Stream
	AverageInt32(opts ...rxgo.Option) Stream
	AverageInt64(opts ...rxgo.Option) Stream
	BackOffRetry(backOffCfg backoff.BackOff, opts ...rxgo.Option) Stream
	BufferWithCount(count int, opts ...rxgo.Option) Stream
	BufferWithTime(milliseconds uint32, opts ...rxgo.Option) Stream
	BufferWithTimeOrCount(milliseconds uint32, count int, opts ...rxgo.Option) Stream
	Connect(ctx context.Context) (context.Context, rxgo.Disposable)
	Contains(equal rxgo.Predicate, opts ...rxgo.Option) Stream
	Count(opts ...rxgo.Option) Stream
	Debounce(milliseconds uint32, opts ...rxgo.Option) Stream
	DefaultIfEmpty(defaultValue interface{}, opts ...rxgo.Option) Stream
	Distinct(apply rxgo.Func, opts ...rxgo.Option) Stream
	DistinctUntilChanged(apply rxgo.Func, opts ...rxgo.Option) Stream
	DoOnCompleted(completedFunc rxgo.CompletedFunc, opts ...rxgo.Option) rxgo.Disposed
	DoOnError(errFunc rxgo.ErrFunc, opts ...rxgo.Option) rxgo.Disposed
	DoOnNext(nextFunc rxgo.NextFunc, opts ...rxgo.Option) rxgo.Disposed
	ElementAt(index uint, opts ...rxgo.Option) Stream
	Error(opts ...rxgo.Option) error
	Errors(opts ...rxgo.Option) []error
	Filter(apply rxgo.Predicate, opts ...rxgo.Option) Stream
	Find(find rxgo.Predicate, opts ...rxgo.Option) Stream
	First(opts ...rxgo.Option) Stream
	FirstOrDefault(defaultValue interface{}, opts ...rxgo.Option) Stream
	FlatMap(apply rxgo.ItemToObservable, opts ...rxgo.Option) Stream
	ForEach(nextFunc rxgo.NextFunc, errFunc rxgo.ErrFunc, completedFunc rxgo.CompletedFunc, opts ...rxgo.Option) rxgo.Disposed
	GroupBy(length int, distribution func(rxgo.Item) int, opts ...rxgo.Option) Stream
	GroupByDynamic(distribution func(rxgo.Item) string, opts ...rxgo.Option) Stream
	IgnoreElements(opts ...rxgo.Option) Stream
	Join(joiner rxgo.Func2, right rxgo.Observable, timeExtractor func(interface{}) time.Time, windowInMS uint32, opts ...rxgo.Option) Stream
	Last(opts ...rxgo.Option) Stream
	LastOrDefault(defaultValue interface{}, opts ...rxgo.Option) Stream
	Map(apply rxgo.Func, opts ...rxgo.Option) Stream
	// Marshal transforms the items emitted by an Observable by applying a marshalling to each item.
	Marshal(marshaller decoder.Marshaller, opts ...rxgo.Option) Stream
	Max(comparator rxgo.Comparator, opts ...rxgo.Option) Stream
	Min(comparator rxgo.Comparator, opts ...rxgo.Option) Stream
	OnErrorResumeNext(resumeSequence rxgo.ErrorToObservable, opts ...rxgo.Option) Stream
	OnErrorReturn(resumeFunc rxgo.ErrorFunc, opts ...rxgo.Option) Stream
	OnErrorReturnItem(resume interface{}, opts ...rxgo.Option) Stream
	Reduce(apply rxgo.Func2, opts ...rxgo.Option) Stream
	Repeat(count int64, milliseconds uint32, opts ...rxgo.Option) Stream
	Retry(count int, shouldRetry func(error) bool, opts ...rxgo.Option) Stream
	Run(opts ...rxgo.Option) rxgo.Disposed
	Sample(iterable rxgo.Iterable, opts ...rxgo.Option) Stream
	Scan(apply rxgo.Func2, opts ...rxgo.Option) Stream
	SequenceEqual(iterable rxgo.Iterable, opts ...rxgo.Option) Stream
	Send(output chan<- rxgo.Item, opts ...rxgo.Option)
	Serialize(from int, identifier func(interface{}) int, opts ...rxgo.Option) Stream
	Skip(nth uint, opts ...rxgo.Option) Stream
	SkipLast(nth uint, opts ...rxgo.Option) Stream
	SkipWhile(apply rxgo.Predicate, opts ...rxgo.Option) Stream
	StartWith(iterable rxgo.Iterable, opts ...rxgo.Option) Stream
	SumFloat32(opts ...rxgo.Option) Stream
	SumFloat64(opts ...rxgo.Option) Stream
	SumInt64(opts ...rxgo.Option) Stream
	Take(nth uint, opts ...rxgo.Option) Stream
	TakeLast(nth uint, opts ...rxgo.Option) Stream
	TakeUntil(apply rxgo.Predicate, opts ...rxgo.Option) Stream
	TakeWhile(apply rxgo.Predicate, opts ...rxgo.Option) Stream
	TimeInterval(opts ...rxgo.Option) Stream
	Timestamp(opts ...rxgo.Option) Stream
	ToMap(keySelector rxgo.Func, opts ...rxgo.Option) Stream
	ToMapWithValueSelector(keySelector, valueSelector rxgo.Func, opts ...rxgo.Option) Stream
	ToSlice(initialCapacity int, opts ...rxgo.Option) ([]interface{}, error)
	// Unmarshal transforms the items emitted by an Observable by applying an unmarshalling to each item.
	Unmarshal(unmarshaller decoder.Unmarshaller, factory func() interface{}, opts ...rxgo.Option) Stream
	WindowWithCount(count int, opts ...rxgo.Option) Stream
	WindowWithTime(milliseconds uint32, opts ...rxgo.Option) Stream
	WindowWithTimeOrCount(milliseconds uint32, count int, opts ...rxgo.Option) Stream
	ZipFromIterable(iterable rxgo.Iterable, zipper rxgo.Func2, opts ...rxgo.Option) Stream

	// SlidingWindowWithCount buffers the data in the specified sliding window size, the buffered data can be processed in the handler func.
	// It returns the orginal data to Stream, not the buffered slice.
	SlidingWindowWithCount(windowSize int, slideSize int, handler Handler, opts ...rxgo.Option) Stream

	// SlidingWindowWithTime buffers the data in the specified sliding window time in milliseconds, the buffered data can be processed in the handler func.
	// It returns the orginal data to Stream, not the buffered slice.
	SlidingWindowWithTime(windowTimeInMS uint32, slideTimeInMS uint32, handler Handler, opts ...rxgo.Option) Stream

	// ZipMultiObservers subscribes multi Y3 observers, zips the values into a slice and calls the zipper callback when all keys are observed.
	ZipMultiObservers(observers []KeyObserveFunc, zipper func(items []interface{}) (interface{}, error)) Stream
}

// KeyObserveFunc is a pair of subscribed key and onObserve callback.
type KeyObserveFunc struct {
	Key       byte
	OnObserve decoder.OnObserveFunc
}
