package main

import (
	"context"
	"fmt"

	y3 "github.com/yomorun/y3-codec-golang"
	"github.com/yomorun/yomo/core/rx"
)

var store = func(_ context.Context, i interface{}) (interface{}, error) {
	value := i.(float32)
	fmt.Printf("save `%v` to FaunaDB\n", value)
	return value, nil
}

var callback = func(v []byte) (interface{}, error) {
	return y3.ToFloat32(v)
}

// Handler will handle data in Rx way
func Handler(rxstream rx.Stream) rx.Stream {
	stream := rxstream.
		Subscribe(0x14).
		OnObserve(callback).
		Map(store)
	return stream
}
