package main

import (
	"context"
	"fmt"
	"os"

	y3 "github.com/yomorun/y3-codec-golang"
	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/logger"
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

// var callback = func(v []byte) (interface{}, error) {
// 	return y3.ToFloat32(v)
// }

// Handler will handle data in Rx way
// func Handler(rxstream rx.Stream) rx.Stream {
// 	stream := rxstream.
// 		Subscribe(NoiseDataKey).
// 		OnObserve(callback).
// 		Map(computePeek)
// 	return stream
// }

func main() {
	sfn := yomo.NewStreamFunction("Noise-2", yomo.WithZipperAddr("localhost:9000"))
	defer sfn.Close()

	sfn.SetObserveDataID(NoiseDataKey)
	sfn.SetHandler(handler)

	err := sfn.Connect()
	if err != nil {
		logger.Errorf("[fn2] connect err=%v", err)
		os.Exit(1)
	}

	select {}
}

func handler(data []byte) (byte, []byte) {
	v, err := y3.ToFloat32(data)
	if err != nil {
		logger.Errorf("[fn2] y3.ToObject err=%v", err)
		return 0x0, nil
	}
	result, err := computePeek(context.Background(), v)
	if err != nil {
		logger.Errorf("[fn2] computePeek err=%v", err)
		return 0x0, nil
	}
	// encode
	encoder := y3.NewPrimitivePacketEncoder(0x01)
	encoder.SetFloat32Value(result.(float32))
	buf := encoder.Encode()

	return 0x15, buf
}
