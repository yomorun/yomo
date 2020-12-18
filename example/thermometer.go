package main

import (
	"context"
	"fmt"
	"time"

	"github.com/yomorun/yomo/pkg/rx"
)

type Thermometer struct {
	Temperature float32 `yomo:"0x11"` // tem
	Humidity    float32 `yomo:"0x12"` // hum
}

func Handler(rxstream rx.RxStream) rx.RxStream {
	stream := rxstream.Y3Decoder("0x20", &Thermometer{}).AuditTime(time.Second).Map(func(_ context.Context, i interface{}) (interface{}, error) {
		value := *i.(*Thermometer)
		fmt.Println("serverless get:", i, "temperature:", value.Temperature, "humidity:", value.Humidity)
		return value, nil
	}).Timeout(6 * time.Second).StdOut()

	return stream
}
