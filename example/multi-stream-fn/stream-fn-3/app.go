package main

import (
	"fmt"

	y3 "github.com/yomorun/y3-codec-golang"
	"github.com/yomorun/yomo/pkg/rx"
)

// NoiseDataKey represents the Tag of a Y3 encoded data packet.
const NoiseDataKey = 0x10

// ThresholdAverageValue is the threshold of the average value after a sliding window.
const ThresholdAverageValue = 13

// SlidingWindowInMS is the time in milliseconds of the sliding window.
const SlidingWindowInMS uint32 = 1e4

// SlidingTimeInMS is the interval in milliseconds of the sliding.
const SlidingTimeInMS uint32 = 1e3

// NoiseData represents the structure of data
type NoiseData struct {
	Noise float32 `y3:"0x11"`
	Time  int64   `y3:"0x12"`
	From  string  `y3:"0x13"`
}

// Unserialize data to `NoiseData` struct, transfer the noise value to next process
var decode = func(v []byte) (interface{}, error) {
	var mold NoiseData
	err := y3.ToObject(v, &mold)
	if err != nil {
		return nil, err
	}
	mold.Noise = mold.Noise / 10
	return mold.Noise, nil
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
func Handler(rxstream rx.RxStream) rx.RxStream {
	stream := rxstream.
		Subscribe(NoiseDataKey).
		OnObserve(decode).
		StdOut().
		SlidingWindowWithTime(SlidingWindowInMS, SlidingTimeInMS, slidingAvg)

	return stream
}
