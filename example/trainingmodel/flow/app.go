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

var zipper = func(_ context.Context, ia interface{}, ib interface{}) (interface{}, error) {

	return fmt.Sprintf("⚡️ Zip [%s],[%s] -> Value: %f, %f", "dataA", "dataB", ia.(float32), ib.(float32)), nil
}

func Handler(rxstream rx.RxStream) rx.RxStream {
	streamA := rxstream.Subscribe(DataAKey).OnObserve(callback).StdOut()
	streamB := rxstream.Subscribe(DataBKey).OnObserve(callback).StdOut()

	stream := streamA.ZipFromIterable(streamB, zipper).StdOut()
	return stream
}
