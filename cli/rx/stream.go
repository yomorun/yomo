package rx

import (
	"context"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/reactivex/rxgo/v2"
)

// Stream is the interface for RxStream.
type Stream interface {
	rxgo.Iterable

	// PipeBackToZipper write the DataFrame with a specified DataID.
	PipeBackToZipper(dataID byte) Stream

	// RawBytes get the raw bytes in Stream which receives from YoMo-Zipper.
	RawBytes() Stream

	// StdOut writes the value as standard output.
	StdOut(opts ...rxgo.Option) Stream

	// AuditTime ignores values for duration milliseconds, then only emits the most recent value.
	AuditTime(milliseconds uint32, opts ...rxgo.Option) Stream

	// DefaultIfEmptyWithTime emits a default value if didn't receive any values for duration milliseconds.
	DefaultIfEmptyWithTime(milliseconds uint32, defaultValue interface{}, opts ...rxgo.Option) Stream

	// All determines whether all items emitted by an Observable meet some criteria
	All(predicate rxgo.Predicate, opts ...rxgo.Option) Stream

	// AverageFloat32 calculates the average of numbers emitted by an Observable and emits the average float32.
	AverageFloat32(opts ...rxgo.Option) Stream

	// AverageFloat64 calculates the average of numbers emitted by an Observable and emits the average float64.
	AverageFloat64(opts ...rxgo.Option) Stream

	// AverageInt calculates the average of numbers emitted by an Observable and emits the average int.
	AverageInt(opts ...rxgo.Option) Stream

	// AverageInt8 calculates the average of numbers emitted by an Observable and emits the average int8.
	AverageInt8(opts ...rxgo.Option) Stream

	// AverageInt16 calculates the average of numbers emitted by an Observable and emits the average int16.
	AverageInt16(opts ...rxgo.Option) Stream

	// AverageInt32 calculates the average of numbers emitted by an Observable and emits the average int32.
	AverageInt32(opts ...rxgo.Option) Stream

	// AverageInt64 calculates the average of numbers emitted by an Observable and emits the average int64.
	AverageInt64(opts ...rxgo.Option) Stream

	// BackOffRetry implements a backoff retry if a source Observable sends an error, resubscribe to it in the hopes that it will complete without error.
	// Cannot be run in parallel.
	BackOffRetry(backOffCfg backoff.BackOff, opts ...rxgo.Option) Stream

	// BufferWithCount returns an Observable that emits buffers of items it collects
	// from the source Observable.
	// The resulting Observable emits buffers every skip items, each containing a slice of count items.
	// When the source Observable completes or encounters an error,
	// the resulting Observable emits the current buffer and propagates
	// the notification from the source Observable.
	BufferWithCount(count int, opts ...rxgo.Option) Stream

	// BufferWithTime returns an Observable that emits buffers of items it collects from the source
	// Observable. The resulting Observable starts a new buffer periodically, as determined by the
	// timeshift argument. It emits each buffer after a fixed timespan, specified by the timespan argument.
	// When the source Observable completes or encounters an error, the resulting Observable emits
	// the current buffer and propagates the notification from the source Observable.
	BufferWithTime(milliseconds uint32, opts ...rxgo.Option) Stream

	// BufferWithTimeOrCount returns an Observable that emits buffers of items it collects from the source
	// Observable either from a given count or at a given time interval.
	BufferWithTimeOrCount(milliseconds uint32, count int, opts ...rxgo.Option) Stream

	// Connect instructs a connectable Observable to begin emitting items to its subscribers.
	Connect(ctx context.Context) (context.Context, rxgo.Disposable)

	// Contains determines whether an Observable emits a particular item or not.
	Contains(equal rxgo.Predicate, opts ...rxgo.Option) Stream

	// Count counts the number of items emitted by the source Observable and emit only this value.
	Count(opts ...rxgo.Option) Stream

	// Debounce only emits an item from an Observable if a particular timespan has passed without it emitting another item.
	Debounce(milliseconds uint32, opts ...rxgo.Option) Stream

	// DefaultIfEmpty returns an Observable that emits the items emitted by the source
	// Observable or a specified default item if the source Observable is empty.
	DefaultIfEmpty(defaultValue interface{}, opts ...rxgo.Option) Stream

	// Distinct suppresses duplicate items in the original Observable and returns
	// a new Observable.
	Distinct(apply rxgo.Func, opts ...rxgo.Option) Stream

	// DistinctUntilChanged suppresses consecutive duplicate items in the original Observable.
	// Cannot be run in parallel.
	DistinctUntilChanged(apply rxgo.Func, opts ...rxgo.Option) Stream

	// DoOnCompleted registers a callback action that will be called once the Observable terminates.
	DoOnCompleted(completedFunc rxgo.CompletedFunc, opts ...rxgo.Option) rxgo.Disposed

	// DoOnError registers a callback action that will be called if the Observable terminates abnormally.
	DoOnError(errFunc rxgo.ErrFunc, opts ...rxgo.Option) rxgo.Disposed

	// DoOnNext registers a callback action that will be called on each item emitted by the Observable.
	DoOnNext(nextFunc rxgo.NextFunc, opts ...rxgo.Option) rxgo.Disposed

	// ElementAt emits only item n emitted by an Observable.
	// Cannot be run in parallel.
	ElementAt(index uint, opts ...rxgo.Option) Stream

	// Error returns the eventual Observable error.
	// This method is blocking.
	Error(opts ...rxgo.Option) error

	// Errors returns an eventual list of Observable errors.
	// This method is blocking
	Errors(opts ...rxgo.Option) []error

	// Filter emits only those items from an Observable that pass a predicate test.
	Filter(apply rxgo.Predicate, opts ...rxgo.Option) Stream

	// Find emits the first item passing a predicate then complete.
	Find(find rxgo.Predicate, opts ...rxgo.Option) Stream

	// First returns new Observable which emit only first item.
	// Cannot be run in parallel.
	First(opts ...rxgo.Option) Stream

	// FirstOrDefault returns new Observable which emit only first item.
	// If the observable fails to emit any items, it emits a default value.
	// Cannot be run in parallel.
	FirstOrDefault(defaultValue interface{}, opts ...rxgo.Option) Stream

	// FlatMap transforms the items emitted by an Observable into Observables, then flatten the emissions from those into a single Observable.
	FlatMap(apply rxgo.ItemToObservable, opts ...rxgo.Option) Stream

	// ForEach subscribes to the Observable and receives notifications for each element.
	ForEach(nextFunc rxgo.NextFunc, errFunc rxgo.ErrFunc, completedFunc rxgo.CompletedFunc, opts ...rxgo.Option) rxgo.Disposed

	// GroupBy divides an Observable into a set of Observables that each emit a different group of items from the original Observable, organized by key.
	GroupBy(length int, distribution func(rxgo.Item) int, opts ...rxgo.Option) Stream

	// GroupByDynamic divides an Observable into a dynamic set of Observables that each emit GroupedObservable from the original Observable, organized by key.
	GroupByDynamic(distribution func(rxgo.Item) string, opts ...rxgo.Option) Stream

	// IgnoreElements ignores all items emitted by the source ObservableSource except for the errors.
	// Cannot be run in parallel.
	IgnoreElements(opts ...rxgo.Option) Stream

	// Join combines items emitted by two Observables whenever an item from one Observable is emitted during
	// a time window defined according to an item emitted by the other Observable.
	// The time is extracted using a timeExtractor function.
	Join(joiner rxgo.Func2, right rxgo.Observable, timeExtractor func(interface{}) time.Time, windowInMS uint32, opts ...rxgo.Option) Stream

	// Last returns a new Observable which emit only last item.
	// Cannot be run in parallel.
	Last(opts ...rxgo.Option) Stream

	// LastOrDefault returns a new Observable which emit only last item.
	// If the observable fails to emit any items, it emits a default value.
	// Cannot be run in parallel.
	LastOrDefault(defaultValue interface{}, opts ...rxgo.Option) Stream

	// Map transforms the items emitted by an Observable by applying a function to each item.
	Map(apply rxgo.Func, opts ...rxgo.Option) Stream

	// Marshal transforms the items emitted by an Observable by applying a marshalling to each item.
	Marshal(marshaller Marshaller, opts ...rxgo.Option) Stream

	// Max determines and emits the maximum-valued item emitted by an Observable according to a comparator.
	Max(comparator rxgo.Comparator, opts ...rxgo.Option) Stream

	// Min determines and emits the minimum-valued item emitted by an Observable according to a comparator.
	Min(comparator rxgo.Comparator, opts ...rxgo.Option) Stream

	// OnErrorResumeNext instructs an Observable to pass control to another Observable rather than invoking
	// onError if it encounters an error.
	OnErrorResumeNext(resumeSequence rxgo.ErrorToObservable, opts ...rxgo.Option) Stream

	// OnErrorReturn instructs an Observable to emit an item (returned by a specified function)
	// rather than invoking onError if it encounters an error.
	OnErrorReturn(resumeFunc rxgo.ErrorFunc, opts ...rxgo.Option) Stream

	// OnErrorReturnItem instructs on Observable to emit an item if it encounters an error.
	OnErrorReturnItem(resume interface{}, opts ...rxgo.Option) Stream

	// Reduce applies a function to each item emitted by an Observable, sequentially, and emit the final value.
	Reduce(apply rxgo.Func2, opts ...rxgo.Option) Stream

	// Repeat returns an Observable that repeats the sequence of items emitted by the source Observable
	// at most count times, at a particular frequency.
	// Cannot run in parallel.
	Repeat(count int64, milliseconds uint32, opts ...rxgo.Option) Stream

	// Retry retries if a source Observable sends an error, resubscribe to it in the hopes that it will complete without error.
	// Cannot be run in parallel.
	Retry(count int, shouldRetry func(error) bool, opts ...rxgo.Option) Stream

	// Run creates an Observer without consuming the emitted items.
	Run(opts ...rxgo.Option) rxgo.Disposed

	// Sample returns an Observable that emits the most recent items emitted by the source
	// Iterable whenever the input Iterable emits an item.
	Sample(iterable rxgo.Iterable, opts ...rxgo.Option) Stream

	// Scan apply a Func2 to each item emitted by an Observable, sequentially, and emit each successive value.
	// Cannot be run in parallel.
	Scan(apply rxgo.Func2, opts ...rxgo.Option) Stream

	// SequenceEqual emits true if an Observable and the input Observable emit the same items,
	// in the same order, with the same termination state. Otherwise, it emits false.
	SequenceEqual(iterable rxgo.Iterable, opts ...rxgo.Option) Stream

	// Send sends the items to a given channel.
	Send(output chan<- rxgo.Item, opts ...rxgo.Option)

	// Serialize forces an Observable to make serialized calls and to be well-behaved.
	Serialize(from int, identifier func(interface{}) int, opts ...rxgo.Option) Stream

	// Skip suppresses the first n items in the original Observable and
	// returns a new Observable with the rest items.
	// Cannot be run in parallel.
	Skip(nth uint, opts ...rxgo.Option) Stream

	// SkipLast suppresses the last n items in the original Observable and
	// returns a new Observable with the rest items.
	// Cannot be run in parallel.
	SkipLast(nth uint, opts ...rxgo.Option) Stream

	// SkipWhile discard items emitted by an Observable until a specified condition becomes false.
	// Cannot be run in parallel.
	SkipWhile(apply rxgo.Predicate, opts ...rxgo.Option) Stream

	// StartWith emits a specified Iterable before beginning to emit the items from the source Observable.
	StartWith(iterable rxgo.Iterable, opts ...rxgo.Option) Stream

	// SumFloat32 calculates the average of float32 emitted by an Observable and emits a float32.
	SumFloat32(opts ...rxgo.Option) Stream

	// SumFloat64 calculates the average of float64 emitted by an Observable and emits a float64.
	SumFloat64(opts ...rxgo.Option) Stream

	// SumInt64 calculates the average of integers emitted by an Observable and emits an int64.
	SumInt64(opts ...rxgo.Option) Stream

	// Take emits only the first n items emitted by an Observable.
	// Cannot be run in parallel.
	Take(nth uint, opts ...rxgo.Option) Stream

	// TakeLast emits only the last n items emitted by an Observable.
	// Cannot be run in parallel.
	TakeLast(nth uint, opts ...rxgo.Option) Stream

	// TakeUntil returns an Observable that emits items emitted by the source Observable,
	// checks the specified predicate for each item, and then completes when the condition is satisfied.
	// Cannot be run in parallel.
	TakeUntil(apply rxgo.Predicate, opts ...rxgo.Option) Stream

	// TakeWhile returns an Observable that emits items emitted by the source ObservableSource so long as each
	// item satisfied a specified condition, and then completes as soon as this condition is not satisfied.
	// Cannot be run in parallel.
	TakeWhile(apply rxgo.Predicate, opts ...rxgo.Option) Stream

	// TimeInterval converts an Observable that emits items into one that emits indications of the amount of time elapsed between those emissions.
	TimeInterval(opts ...rxgo.Option) Stream

	// Timestamp attaches a timestamp to each item emitted by an Observable indicating when it was emitted.
	Timestamp(opts ...rxgo.Option) Stream

	// ToMap convert the sequence of items emitted by an Observable
	// into a map keyed by a specified key function.
	// Cannot be run in parallel.
	ToMap(keySelector rxgo.Func, opts ...rxgo.Option) Stream

	// ToMapWithValueSelector convert the sequence of items emitted by an Observable
	// into a map keyed by a specified key function and valued by another
	// value function.
	// Cannot be run in parallel.
	ToMapWithValueSelector(keySelector, valueSelector rxgo.Func, opts ...rxgo.Option) Stream

	// ToSlice collects all items from an Observable and emit them in a slice and an optional error.
	// Cannot be run in parallel.
	ToSlice(initialCapacity int, opts ...rxgo.Option) ([]interface{}, error)

	// Unmarshal transforms the items emitted by an Observable by applying an unmarshalling to each item.
	Unmarshal(unmarshaller Unmarshaller, factory func() interface{}, opts ...rxgo.Option) Stream

	// WindowWithCount periodically subdivides items from an Observable into Observable windows of a given size and emit these windows
	// rather than emitting the items one at a time.
	WindowWithCount(count int, opts ...rxgo.Option) Stream

	// WindowWithTime periodically subdivides items from an Observable into Observables based on timed windows
	// and emit them rather than emitting the items one at a time.
	WindowWithTime(milliseconds uint32, opts ...rxgo.Option) Stream

	// WindowWithTimeOrCount periodically subdivides items from an Observable into Observables based on timed windows or a specific size
	// and emit them rather than emitting the items one at a time.
	WindowWithTimeOrCount(milliseconds uint32, count int, opts ...rxgo.Option) Stream

	// ZipFromIterable merges the emissions of an Iterable via a specified function
	// and emit single items for each combination based on the results of this function.
	ZipFromIterable(iterable rxgo.Iterable, zipper rxgo.Func2, opts ...rxgo.Option) Stream

	// SlidingWindowWithCount buffers the data in the specified sliding window size, the buffered data can be processed in the handler func.
	// It returns the orginal data to Stream, not the buffered slice.
	SlidingWindowWithCount(windowSize int, slideSize int, handler Handler, opts ...rxgo.Option) Stream

	// SlidingWindowWithTime buffers the data in the specified sliding window time in milliseconds, the buffered data can be processed in the handler func.
	// It returns the orginal data to Stream, not the buffered slice.
	SlidingWindowWithTime(windowTimeInMS uint32, slideTimeInMS uint32, handler Handler, opts ...rxgo.Option) Stream

	// // ZipMultiObservers subscribes multi Y3 observers, zips the values into a slice and calls the zipper callback when all keys are observed.
	// ZipMultiObservers(observers []KeyObserveFunc, zipper func(items []interface{}) (interface{}, error)) Stream
}

// // KeyObserveFunc is a pair of subscribed key and onObserve callback.
// type KeyObserveFunc struct {
// 	Key       byte
// 	OnObserve decoder.OnObserveFunc
// }

type (
	// Marshaller defines a marshaller type (interface{} to []byte).
	Marshaller func(interface{}) ([]byte, error)
	// Unmarshaller defines an unmarshaller type ([]byte to interface).
	Unmarshaller func([]byte, interface{}) error

	// BytesWithDataID is the bytes with a specific DataID to pipe back to zipper.
	BytesWithDataID struct {
		// DataID to identify the bytes when pipe back to zipper.
		DataID byte
		// Bytes of the new data.
		Bytes []byte
	}
)
