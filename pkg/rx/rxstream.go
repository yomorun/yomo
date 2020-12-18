package rx

import (
	"context"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/reactivex/rxgo/v2"
)

type RxStream interface {
	rxgo.Iterable
	Y3Decoder(key string, mold interface{}, opts ...rxgo.Option) RxStream
	StdOut(opts ...rxgo.Option) RxStream
	AuditTime(timespan time.Duration, opts ...rxgo.Option) RxStream
	Timeout(timespan time.Duration, opts ...rxgo.Option) RxStream
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
	BufferWithTime(timespan rxgo.Duration, opts ...rxgo.Option) RxStream
	BufferWithTimeOrCount(timespan rxgo.Duration, count int, opts ...rxgo.Option) RxStream
	Connect(ctx context.Context) (context.Context, rxgo.Disposable)
	Contains(equal rxgo.Predicate, opts ...rxgo.Option) RxStream
	Count(opts ...rxgo.Option) RxStream
	Debounce(timespan rxgo.Duration, opts ...rxgo.Option) RxStream
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
	Join(joiner rxgo.Func2, right rxgo.Observable, timeExtractor func(interface{}) time.Time, window rxgo.Duration, opts ...rxgo.Option) RxStream
	Last(opts ...rxgo.Option) RxStream
	LastOrDefault(defaultValue interface{}, opts ...rxgo.Option) RxStream
	Map(apply rxgo.Func, opts ...rxgo.Option) RxStream
	Marshal(marshaller rxgo.Marshaller, opts ...rxgo.Option) RxStream
	Max(comparator rxgo.Comparator, opts ...rxgo.Option) RxStream
	Min(comparator rxgo.Comparator, opts ...rxgo.Option) RxStream
	OnErrorResumeNext(resumeSequence rxgo.ErrorToObservable, opts ...rxgo.Option) RxStream
	OnErrorReturn(resumeFunc rxgo.ErrorFunc, opts ...rxgo.Option) RxStream
	OnErrorReturnItem(resume interface{}, opts ...rxgo.Option) RxStream
	Reduce(apply rxgo.Func2, opts ...rxgo.Option) RxStream
	Repeat(count int64, frequency rxgo.Duration, opts ...rxgo.Option) RxStream
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
	Unmarshal(unmarshaller rxgo.Unmarshaller, factory func() interface{}, opts ...rxgo.Option) RxStream
	WindowWithCount(count int, opts ...rxgo.Option) RxStream
	WindowWithTime(timespan rxgo.Duration, opts ...rxgo.Option) RxStream
	WindowWithTimeOrCount(timespan rxgo.Duration, count int, opts ...rxgo.Option) RxStream
	ZipFromIterable(iterable rxgo.Iterable, zipper rxgo.Func2, opts ...rxgo.Option) RxStream
}
