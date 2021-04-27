package main

import (
	"context"
	"fmt"

	"github.com/yomorun/y3-codec-golang"
	"github.com/yomorun/yomo/pkg/rx"
)

var zipper = func(_ context.Context, a interface{}, b interface{}) (interface{}, error) {
	accumulator, ok := a.([]int64)
	if !ok {
		fmt.Printf("No accumulator: %v + %v\n", a, b)
		return []int64{a.(int64), b.(int64)}, nil
	}

	fmt.Printf("With accumulator: %v + %v\n", accumulator, b)
	accumulator = append(accumulator, b.(int64))
	return accumulator, nil
}

var convert = func(v []byte) (interface{}, error) {
	return y3.ToInt64(v)
}

// Handler will handle data in Rx way
func Handler(rxstream rx.RxStream) rx.RxStream {
	streamA := rxstream.Subscribe(0x10).OnObserve(convert)
	streamB := rxstream.Subscribe(0x11).OnObserve(convert)
	streamC := rxstream.Subscribe(0x12).OnObserve(convert)
	streamD := rxstream.Subscribe(0x13).OnObserve(convert)
	streamE := rxstream.Subscribe(0x14).OnObserve(convert)

	return streamA.
		ZipFromIterable(streamB, zipper).
		ZipFromIterable(streamC, zipper).
		ZipFromIterable(streamD, zipper).
		ZipFromIterable(streamE, zipper).
		StdOut().
		Encode(0x11)
}
