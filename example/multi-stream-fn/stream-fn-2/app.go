package main

import (
	"context"
	"fmt"

	y3 "github.com/yomorun/y3-codec-golang"
	"github.com/yomorun/yomo/rx"
)

// NoiseDataKey represents the Tag of a Y3 encoded data packet.
const NoiseDataKey = 0x14

// ThresholdSingleValue is the threshold of a single value.
const ThresholdSingleValue = 16

// Print every value and alert for value greater than ThresholdSingleValue
var computePeek = func(_ context.Context, i interface{}) (interface{}, error) {
	value := i.(float32)

	fmt.Println(fmt.Sprintf("receive noise value: %f", value))

	// Compute peek value, if greater than ThresholdSingleValue, alert
	if value >= ThresholdSingleValue {
		fmt.Println(fmt.Sprintf("❗ value: %f reaches the threshold %d! 𝚫=%f", value, ThresholdSingleValue, value-ThresholdSingleValue))
	}

	return value, nil
}

var callback = func(v []byte) (interface{}, error) {
	return y3.ToFloat32(v)
}

// Handler will handle data in Rx way
func Handler(rxstream rx.Stream) rx.Stream {
	stream := rxstream.
		Subscribe(NoiseDataKey).
		OnObserve(callback).
		Map(computePeek)
	return stream
}
