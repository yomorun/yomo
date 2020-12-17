package main

import (
	"context"
	"fmt"
	"time"

	"github.com/yomorun/yomo/pkg/rx"
)

func Handler(rxstream rx.RxStream) rx.RxStream {
	stream := rxstream.AuditTime(time.Second).Map(func(_ context.Context, i interface{}) (interface{}, error) {
		fmt.Println("serverless get:", i, "string:", string(i.([]byte)))
		return string(i.([]byte)), nil
	}).Timeout(6 * time.Second).StdOut()

	return stream
}
