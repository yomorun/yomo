package main

import (
	"context"
	"fmt"
	"time"

	"github.com/yomorun/yomo/pkg/rx"
)

var store = func(_ context.Context, i interface{}) (interface{}, error) {
	value := i.(float32)
	fmt.Printf("save `%v` to FaunaDB\n", value)
	return value, nil
}

// Handler will handle data in Rx way
func Handler(rxstream rx.RxStream) rx.RxStream {
	stream := rxstream.
		Y3Decoder("0x10", float32(0)).
		AuditTime(100 * time.Millisecond).
		Map(store)
	return stream
}
