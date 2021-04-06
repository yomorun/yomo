package main

import (
	"context"
	"fmt"
	"time"

	"github.com/reactivex/rxgo/v2"
	y3 "github.com/yomorun/y3-codec-golang"
	"github.com/yomorun/yomo/pkg/rx"
)

// NoiseDataKey represents the Tag of a Y3 encoded data packet.
const NoiseDataKey = 0x10

// ThresholdSingleValue is the threshold of a single value.
const ThresholdSingleValue = 16

// ThresholdAverageValue is the threshold of the average value after a sliding window.
const ThresholdAverageValue = 13

// SlidingWindowSeconds is the time in seconds of the sliding window.
const SlidingWindowSeconds = 10

// NoiseData represents the structure of data
type NoiseData struct {
	Noise float32 `y3:"0x11"`
	Time  int64   `y3:"0x12"`
	From  string  `y3:"0x13"`
}

// Print every value and alert for value greater than ThresholdSingleValue
var computePeek = func(_ context.Context, i interface{}) (interface{}, error) {
	value := i.(NoiseData)
	rightNow := time.Now().UnixNano() / int64(time.Millisecond)
	fmt.Println(fmt.Sprintf("[%s] %d > value: %f ⚡️=%dms", value.From, value.Time, value.Noise, rightNow-value.Time))

	// Compute peek value, if greater than ThresholdSingleValue, alert
	if value.Noise >= ThresholdSingleValue {
		fmt.Println(fmt.Sprintf("❗ value: %f reaches the threshold %d! 𝚫=%f", value.Noise, ThresholdSingleValue, value.Noise-ThresholdSingleValue))
	}

	return value.Noise, nil
}

// Unserialize data to `NoiseData` struct, transfer to next process
var decode = func(v []byte) (interface{}, error) {
	var mold NoiseData
	err := y3.ToObject(v, &mold)
	if err != nil {
		return nil, err
	}
	mold.Noise = mold.Noise / 10
	return mold, nil
}

// Compute avg of every past 10-seconds IoT data
var slidingAvg = func(i interface{}) error {
	values, ok := i.([]interface{})
	if ok {
		var total float32 = 0
		for _, value := range values {
			total += value.(float32)
		}
		avg := total / float32(len(values))
		fmt.Println(fmt.Sprintf("🧩 average value in last %d seconds: %f!", SlidingWindowSeconds, avg))
		if avg >= ThresholdAverageValue {
			fmt.Println(fmt.Sprintf("❗❗  average value in last %d seconds: %f reaches the threshold %d!", SlidingWindowSeconds, avg, ThresholdAverageValue))
		}
	}
	return nil
}

// Handler will handle data in Rx way
func Handler(rxstream rx.RxStream) rx.RxStream {
	stream := rxstream.
		Subscribe(NoiseDataKey).
		Debounce(rxgo.WithDuration(50*time.Millisecond)).
		OnObserve(decode).
		Map(computePeek).
		SlidingWindowWithTime(SlidingWindowSeconds*time.Second, 1*time.Second, slidingAvg).
		Encode(0x11)

	return stream
}
