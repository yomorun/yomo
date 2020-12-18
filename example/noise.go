package main

import (
	"context"
	"fmt"
	"time"

	"github.com/yomorun/yomo/pkg/rx"
)

func Handler(rxstream rx.RxStream) rx.RxStream {
	stream := rxstream.Y3Decoder("0x10", float32(0)).AuditTime(100 * time.Millisecond).Map(func(_ context.Context, i interface{}) (interface{}, error) {
		value := i.(float32)
		fmt.Println("serverless get value:", value)
		return value, nil
	}).StdOut()

	return stream
}
