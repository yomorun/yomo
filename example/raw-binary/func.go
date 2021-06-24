package main

import (
	"fmt"

	"github.com/yomorun/yomo/pkg/rx"
)

func Handler(rx rx.RxStream) rx.RxStream {
	return rx.Subscribe(0x10).
	OnObserve(f).
	StdOut().
	Encode(0x11)
}

var f = func(v []byte) (interface{}, error) {
	return fmt.Sprintf("f.v=%v", v), nil
}
