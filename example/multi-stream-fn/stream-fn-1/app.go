package main

import (
	"context"
	"os"
	"time"

	y3 "github.com/yomorun/y3-codec-golang"
	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/logger"
)

// NoiseDataKey represents the Tag of a Y3 encoded data packet.
const NoiseDataKey = 0x10

// NoiseData represents the structure of data
type NoiseData struct {
	Noise float32 `y3:"0x11"`
	Time  int64   `y3:"0x12"`
	From  string  `y3:"0x13"`
}

// Print every value and return noise value to downstream.
var print = func(_ context.Context, i interface{}) (interface{}, error) {
	value := i.(NoiseData)
	rightNow := time.Now().UnixNano() / int64(time.Millisecond)
	// fmt.Println(fmt.Sprintf("[%s] %d > value: %f ⚡️=%dms", value.From, value.Time, value.Noise, rightNow-value.Time))
	logger.Printf("[%s] %d > value: %f ⚡️=%dms", value.From, value.Time, value.Noise, rightNow-value.Time)

	return value.Noise, nil
}

// Unserialize data to `NoiseData` struct, transfer to next process
// var decode = func(v []byte) (interface{}, error) {
// 	var mold NoiseData
// 	err := y3.ToObject(v, &mold)
// 	if err != nil {
// 		return nil, err
// 	}
// 	mold.Noise = mold.Noise / 10
// 	return mold, nil
// }

// Handler will handle data in Rx way
// func Handler(rxstream rx.Stream) rx.Stream {
// 	stream := rxstream.
// 		Subscribe(NoiseDataKey).
// 		OnObserve(decode).
// 		Map(print).
// 		Encode(0x14)

// 	return stream
// }

func main() {
	sfn := yomo.NewStreamFunction("Noise-1", yomo.WithZipperAddr("localhost:9000"))
	defer sfn.Close()

	sfn.SetObserveDataID(NoiseDataKey)
	sfn.SetHandler(handler)

	err := sfn.Connect()
	if err != nil {
		logger.Errorf("[fn1] connect err=%v", err)
		os.Exit(1)
	}

	select {}
}

func handler(data []byte) (byte, []byte) {
	var mold NoiseData
	err := y3.ToObject(data, &mold)
	if err != nil {
		logger.Errorf("[fn1] y3.ToObject err=%v", err)
		return 0x0, nil
	}
	mold.Noise = mold.Noise / 10
	// Print every value and return noise value to downstream.
	result, err := print(context.Background(), mold)
	if err != nil {
		logger.Errorf("[fn1] to downstream err=%v", err)
		return 0x0, nil
	}
	// encode
	buf, _ := y3.NewCodec(0x20).Marshal(result)

	return 0x14, buf
}
