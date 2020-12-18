package main

import (
	"context"
	"fmt"
	"time"

	"github.com/yomorun/yomo/pkg/rx"
)

var printer = func(_ context.Context, i interface{}) (interface{}, error) {
	value := i.(float32)
	fmt.Println("serverless get value:", value)
	return value, nil
}

// Handler will handle data in Rx way
func Handler(rxstream rx.RxStream) rx.RxStream {
	stream := rxstream.
		Y3Decoder("0x10", float32(0)).
		AuditTime(100 * time.Millisecond).
		Map(printer).
		StdOut()

	return stream
}
