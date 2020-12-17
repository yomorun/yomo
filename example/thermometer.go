package main

import (
	"context"
	"fmt"
	"time"

	"github.com/yomorun/yomo/pkg/rx"
)

type Thermometer struct {
	Id          string  `yomo:"0x10"` // id
	Temperature float32 `yomo:"0x11"` // tem
	Humidity    float32 `yomo:"0x12"` // hum
	Stored      bool    `yomo:"0x13"` // stored
}

func Handler(rxstream rx.RxStream) rx.RxStream {
	stream := rxstream.Y3Decoder("0x20", func() interface{} {
		return &[]Thermometer{}
	}).AuditTime(time.Second).Map(func(_ context.Context, i interface{}) (interface{}, error) {
		fmt.Println("serverless get:", i, "value:", *value.(*[]Thermometer))
		return *value.(*[]Thermometer), nil
	}).Timeout(6 * time.Second).StdOut()

	return stream
}
