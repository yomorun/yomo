package main

import (
	"context"
	"fmt"
	"time"

	y3 "github.com/yomorun/y3-codec-golang"
	"github.com/yomorun/yomo/pkg/rx"
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
func Handler(rxstream rx.RxStream) rx.RxStream {
	stream := rxstream.
		Subscribe(0x11).
		OnObserve(callback).
		AuditTime(100 * time.Millisecond).
		Map(store).
		Encode(0x11)
	return stream
}
