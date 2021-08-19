package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/yomorun/yomo/core/rx"
)

// NoiseDataKey represents the Tag of a Y3 encoded data packet.
const NoiseDataKey = 0x10

// ThresholdSingleValue is the threshold of a single value.
const ThresholdSingleValue = 16

// ThresholdAverageValue is the threshold of the average value after a sliding window.
const ThresholdAverageValue = 13

// SlidingWindowInMS is the time in milliseconds of the sliding window.
const SlidingWindowInMS uint32 = 1e4

// SlidingTimeInMS is the interval in milliseconds of the sliding.
const SlidingTimeInMS uint32 = 1e3

// NoiseData represents the structure of data
type NoiseData struct {
	Noise float32 `json:"noise"`
	Time  int64   `json:"time"`
	From  string  `json:"from"`
}

// Print every value and alert for value greater than ThresholdSingleValue
var computePeek = func(_ context.Context, i interface{}) (interface{}, error) {
	value := i.(*NoiseData)
	// Calculate the actual noise value
	value.Noise = value.Noise / 10

	// Calculate latency
	rightNow := time.Now().UnixNano() / int64(time.Millisecond)
	fmt.Println(fmt.Sprintf("[%s] %d > value: %f ⚡️=%dms", value.From, value.Time, value.Noise, rightNow-value.Time))

	// Compute peek value, if greater than ThresholdSingleValue, alert
	if value.Noise >= ThresholdSingleValue {
		fmt.Println(fmt.Sprintf("❗ value: %f reaches the threshold %d! 𝚫=%f", value.Noise, ThresholdSingleValue, value.Noise-ThresholdSingleValue))
	}

	return value.Noise, nil
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
		fmt.Println(fmt.Sprintf("🧩 average value in last %d ms: %f!", SlidingWindowInMS, avg))
		if avg >= ThresholdAverageValue {
			fmt.Println(fmt.Sprintf("❗❗  average value in last %d ms: %f reaches the threshold %d!", SlidingWindowInMS, avg, ThresholdAverageValue))
		}
	}
	return nil
}

// Handler will handle data in Rx way
func Handler(rxstream rx.Stream) rx.Stream {
	stream := rxstream.
		Unmarshal(json.Unmarshal, func() interface{} { return &NoiseData{} }).
		Debounce(50).
		Map(computePeek).
		SlidingWindowWithTime(SlidingWindowInMS, SlidingTimeInMS, slidingAvg).
		Marshal(json.Marshal)

	return stream
}
