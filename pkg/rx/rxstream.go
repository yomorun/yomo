package rx

import (
	"context"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/reactivex/rxgo/v2"
	"github.com/yomorun/yomo/pkg/decoder"
	"github.com/yomorun/yomo/pkg/serverless"
)

type RxStream interface {
	rxgo.Iterable

	// MergeStreamFunc sends the stream data to Stream Function and receives the new stream data from it.
	MergeStreamFunc(sfn serverless.GetStreamFunc, opts ...rxgo.Option) RxStream

	// Subscribe a specified key in stream and gets the data when the key is observed by Y3 Codec.
	Subscribe(key byte) RxStream

	// OnObserve calls the function to process the observed data.
	OnObserve(function func(v []byte) (interface{}, error)) RxStream

	// Encode the data with a specified key by Y3 Codec and append it to stream.
	Encode(key byte, opts ...rxgo.Option) RxStream

	StdOut(opts ...rxgo.Option) RxStream
	AuditTime(milliseconds uint32, opts ...rxgo.Option) RxStream
	DefaultIfEmptyWithTime(milliseconds uint32, defaultValue interface{}, opts ...rxgo.Option) RxStream
	All(predicate rxgo.Predicate, opts ...rxgo.Option) RxStream
	AverageFloat32(opts ...rxgo.Option) RxStream
	AverageFloat64(opts ...rxgo.Option) RxStream
	AverageInt(opts ...rxgo.Option) RxStream
	AverageInt8(opts ...rxgo.Option) RxStream
	AverageInt16(opts ...rxgo.Option) RxStream
	AverageInt32(opts ...rxgo.Option) RxStream
	AverageInt64(opts ...rxgo.Option) RxStream
	BackOffRetry(backOffCfg backoff.BackOff, opts ...rxgo.Option) RxStream
	BufferWithCount(count int, opts ...rxgo.Option) RxStream
	BufferWithTime(milliseconds uint32, opts ...rxgo.Option) RxStream
	BufferWithTimeOrCount(milliseconds uint32, count int, opts ...rxgo.Option) RxStream
	Connect(ctx context.Context) (context.Context, rxgo.Disposable)
	Contains(equal rxgo.Predicate, opts ...rxgo.Option) RxStream
	Count(opts ...rxgo.Option) RxStream
	Debounce(milliseconds uint32, opts ...rxgo.Option) RxStream
	DefaultIfEmpty(defaultValue interface{}, opts ...rxgo.Option) RxStream
	Distinct(apply rxgo.Func, opts ...rxgo.Option) RxStream
	DistinctUntilChanged(apply rxgo.Func, opts ...rxgo.Option) RxStream
	DoOnCompleted(completedFunc rxgo.CompletedFunc, opts ...rxgo.Option) rxgo.Disposed
	DoOnError(errFunc rxgo.ErrFunc, opts ...rxgo.Option) rxgo.Disposed
	DoOnNext(nextFunc rxgo.NextFunc, opts ...rxgo.Option) rxgo.Disposed
	ElementAt(index uint, opts ...rxgo.Option) RxStream
	Error(opts ...rxgo.Option) error
	Errors(opts ...rxgo.Option) []error
	Filter(apply rxgo.Predicate, opts ...rxgo.Option) RxStream
	Find(find rxgo.Predicate, opts ...rxgo.Option) RxStream
	First(opts ...rxgo.Option) RxStream
	FirstOrDefault(defaultValue interface{}, opts ...rxgo.Option) RxStream
	FlatMap(apply rxgo.ItemToObservable, opts ...rxgo.Option) RxStream
	ForEach(nextFunc rxgo.NextFunc, errFunc rxgo.ErrFunc, completedFunc rxgo.CompletedFunc, opts ...rxgo.Option) rxgo.Disposed
	GroupBy(length int, distribution func(rxgo.Item) int, opts ...rxgo.Option) RxStream
	GroupByDynamic(distribution func(rxgo.Item) string, opts ...rxgo.Option) RxStream
	IgnoreElements(opts ...rxgo.Option) RxStream
	Join(joiner rxgo.Func2, right rxgo.Observable, timeExtractor func(interface{}) time.Time, windowInMS uint32, opts ...rxgo.Option) RxStream
	Last(opts ...rxgo.Option) RxStream
	LastOrDefault(defaultValue interface{}, opts ...rxgo.Option) RxStream
	Map(apply rxgo.Func, opts ...rxgo.Option) RxStream
	// Marshal transforms the items emitted by an Observable by applying a marshalling to each item.
	Marshal(marshaller decoder.Marshaller, opts ...rxgo.Option) RxStream
	Max(comparator rxgo.Comparator, opts ...rxgo.Option) RxStream
	Min(comparator rxgo.Comparator, opts ...rxgo.Option) RxStream
	OnErrorResumeNext(resumeSequence rxgo.ErrorToObservable, opts ...rxgo.Option) RxStream
	OnErrorReturn(resumeFunc rxgo.ErrorFunc, opts ...rxgo.Option) RxStream
	OnErrorReturnItem(resume interface{}, opts ...rxgo.Option) RxStream
	Reduce(apply rxgo.Func2, opts ...rxgo.Option) RxStream
	Repeat(count int64, milliseconds uint32, opts ...rxgo.Option) RxStream
	Retry(count int, shouldRetry func(error) bool, opts ...rxgo.Option) RxStream
	Run(opts ...rxgo.Option) rxgo.Disposed
	Sample(iterable rxgo.Iterable, opts ...rxgo.Option) RxStream
	Scan(apply rxgo.Func2, opts ...rxgo.Option) RxStream
	SequenceEqual(iterable rxgo.Iterable, opts ...rxgo.Option) RxStream
	Send(output chan<- rxgo.Item, opts ...rxgo.Option)
	Serialize(from int, identifier func(interface{}) int, opts ...rxgo.Option) RxStream
	Skip(nth uint, opts ...rxgo.Option) RxStream
	SkipLast(nth uint, opts ...rxgo.Option) RxStream
	SkipWhile(apply rxgo.Predicate, opts ...rxgo.Option) RxStream
	StartWith(iterable rxgo.Iterable, opts ...rxgo.Option) RxStream
	SumFloat32(opts ...rxgo.Option) RxStream
	SumFloat64(opts ...rxgo.Option) RxStream
	SumInt64(opts ...rxgo.Option) RxStream
	Take(nth uint, opts ...rxgo.Option) RxStream
	TakeLast(nth uint, opts ...rxgo.Option) RxStream
	TakeUntil(apply rxgo.Predicate, opts ...rxgo.Option) RxStream
	TakeWhile(apply rxgo.Predicate, opts ...rxgo.Option) RxStream
	TimeInterval(opts ...rxgo.Option) RxStream
	Timestamp(opts ...rxgo.Option) RxStream
	ToMap(keySelector rxgo.Func, opts ...rxgo.Option) RxStream
	ToMapWithValueSelector(keySelector, valueSelector rxgo.Func, opts ...rxgo.Option) RxStream
	ToSlice(initialCapacity int, opts ...rxgo.Option) ([]interface{}, error)
	// Unmarshal transforms the items emitted by an Observable by applying an unmarshalling to each item.
	Unmarshal(unmarshaller decoder.Unmarshaller, factory func() interface{}, opts ...rxgo.Option) RxStream
	WindowWithCount(count int, opts ...rxgo.Option) RxStream
	WindowWithTime(milliseconds uint32, opts ...rxgo.Option) RxStream
	WindowWithTimeOrCount(milliseconds uint32, count int, opts ...rxgo.Option) RxStream
	ZipFromIterable(iterable rxgo.Iterable, zipper rxgo.Func2, opts ...rxgo.Option) RxStream

	// SlidingWindowWithCount buffers the data in the specified sliding window size, the buffered data can be processed in the handler func.
	// It returns the orginal data to RxStream, not the buffered slice.
	SlidingWindowWithCount(windowSize int, slideSize int, handler Handler, opts ...rxgo.Option) RxStream

	// SlidingWindowWithTime buffers the data in the specified sliding window time in milliseconds, the buffered data can be processed in the handler func.
	// It returns the orginal data to RxStream, not the buffered slice.
	SlidingWindowWithTime(windowTimeInMS uint32, slideTimeInMS uint32, handler Handler, opts ...rxgo.Option) RxStream

	// ZipMultiObservers subscribes multi Y3 observers, zips the values into a slice and calls the zipper callback when all keys are observed.
	ZipMultiObservers(observers []decoder.KeyObserveFunc, zipper func(items []interface{}) (interface{}, error)) RxStream
}
