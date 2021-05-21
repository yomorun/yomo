package main

import (
	"context"
	"fmt"
	"log"
	"time"

	y3 "github.com/yomorun/y3-codec-golang"
	"github.com/yomorun/yomo/pkg/client"
	"github.com/yomorun/yomo/pkg/rx"
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
		Debounce(50).
		OnObserve(decode).
		Map(computePeek).
		SlidingWindowWithTime(SlidingWindowInMS, SlidingTimeInMS, slidingAvg).
		Encode(0x11)

	return stream
}

func main() {
	cli, err := client.NewServerless("Noise").Connect("localhost", 9000)
	if err != nil {
		log.Print("❌ Connect to zipper failure: ", err)
		return
	}

	defer cli.Close()
	cli.Pipe(Handler)
}
