package main

import (
	"context"
	"fmt"

	y3 "github.com/yomorun/y3-codec-golang"
	"github.com/yomorun/yomo/rx"
)

const dataAKey = 0x11
const dataBKey = 0x12

var convert = func(v []byte) (interface{}, error) {
	return y3.ToFloat32(v)
}

var zipper = func(_ context.Context, ia interface{}, ib interface{}) (interface{}, error) {
	result := ia.(float32) + ib.(float32)
	return fmt.Sprintf("⚡️ Sum(%s: %f, %s: %f) => Result: %f", "data A", ia.(float32), "data B", ib.(float32), result), nil
}

// Handler handle two event streams and calculate sum when data arrived
func Handler(rxstream rx.Stream) rx.Stream {
	streamA := rxstream.Subscribe(dataAKey).OnObserve(convert)
	streamB := rxstream.Subscribe(dataBKey).OnObserve(convert)

	// Rx Zip operator: http://reactivex.io/documentation/operators/zip.html
	stream := streamA.ZipFromIterable(streamB, zipper).StdOut().Encode(0x13)
	return stream
}
