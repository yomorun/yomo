package main

import (
	"context"
	"fmt"
	"time"

	"github.com/reactivex/rxgo/v2"
	y3 "github.com/yomorun/y3-codec-golang"
	"github.com/yomorun/yomo/pkg/rx"
)

// NoiseDataKey represents the Tag of a Y3 encoded data packet
const NoiseDataKey = 0x10

// NoiseData represents the structure of data
type NoiseData struct {
	Noise float32 `y3:"0x11"`
	Time  int64   `y3:"0x12"`
	From  string  `y3:"0x13"`
}

var printer = func(_ context.Context, i interface{}) (interface{}, error) {
	value := i.(NoiseData)
	rightNow := time.Now().UnixNano() / int64(time.Millisecond)
	fmt.Println(fmt.Sprintf("[%s] %d > value: %f ⚡️=%dms", value.From, value.Time, value.Noise, rightNow-value.Time))
	return value.Noise, nil
}

var callback = func(v []byte) (interface{}, error) {
	var mold NoiseData
	err := y3.ToObject(v, &mold)
	if err != nil {
		return nil, err
	}
	mold.Noise = mold.Noise / 10
	return mold, nil
}

// Handler will handle data in Rx way
func Handler(rxstream rx.RxStream) rx.RxStream {
	stream := rxstream.
		Subscribe(NoiseDataKey).
		OnObserve(callback).
		Debounce(rxgo.WithDuration(50 * time.Millisecond)).
		Map(printer).
		StdOut().
		Encode(0x11)

	return stream
}
