package main

import (
	"fmt"

	"github.com/yomorun/y3-codec-golang"
	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/rx"
)

var zipper = func(items []interface{}) (interface{}, error) {
	var result int64 = 0
	for _, item := range items {
		result += item.(int64)
	}
	return fmt.Sprintf("Sum (%v), result: %v", items, result), nil
}

var convert = func(v []byte) (interface{}, error) {
	return y3.ToInt64(v)
}

// Handler will handle data in Rx way
func Handler(rxstream rx.Stream) rx.Stream {
	observers := []yomo.KeyObserveFunc{
		{
			Key:       0x10,
			OnObserve: convert,
		},
		{
			Key:       0x11,
			OnObserve: convert,
		},
		{
			Key:       0x12,
			OnObserve: convert,
		},
		{
			Key:       0x13,
			OnObserve: convert,
		},
		{
			Key:       0x14,
			OnObserve: convert,
		},
	}

	return rxstream.
		ZipMultiObservers(observers, zipper).
		StdOut().
		Encode(0x11)
}
