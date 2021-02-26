package main

import (
	"context"
	"fmt"

	y3 "github.com/yomorun/y3-codec-golang"
	"github.com/yomorun/yomo/pkg/rx"
)

const DataAKey = 0x11
const DataBKey = 0x12

var callback = func(v []byte) (interface{}, error) {
	return y3.ToFloat32(v)
}

var printera = func(_ context.Context, i interface{}) (interface{}, error) {
	value := i.(float32)
	fmt.Println(fmt.Sprintf("[%s]> value: %f", "data-a", value))
	return i, nil
}

var printerb = func(_ context.Context, i interface{}) (interface{}, error) {
	value := i.(float32)
	fmt.Println(fmt.Sprintf("[%s]> value: %f", "data-b", value))
	return i, nil
}

var zipper = func(_ context.Context, ia interface{}, ib interface{}) (interface{}, error) {

	return fmt.Sprintf("⚡️ Zip [%s],[%s] -> Value: %f, %f", "dataA", "dataB", ia.(float32), ib.(float32)), nil
}

func Handler(rxstream rx.RxStream) rx.RxStream {
	streamA := rxstream.Subscribe(DataAKey).OnObserve(callback).Map(printera)
	streamB := rxstream.Subscribe(DataBKey).OnObserve(callback).Map(printerb)

	stream := streamA.ZipFromIterable(streamB, zipper).StdOut().Encode(0x10)
	return stream
}
