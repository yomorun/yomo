package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/yomorun/yomo/core/rx"
)

// NoiseData represents the structure of data
type NoiseData struct {
	Noise float32 `json:"noise"` // Noise value
	Time  int64   `json:"time"` // Timestamp (ms)
	From  string  `json:"from"` // Source IP
}

var store = func(_ context.Context, i interface{}) (interface{}, error) {
	value := i.(*NoiseData)
	fmt.Printf("save `%v` to FaunaDB\n", value.Noise)
	return value, nil
}

// Handler will handle data in Rx way
func Handler(rxstream rx.Stream) rx.Stream {
	stream := rxstream.
	  Unmarshal(json.Unmarshal, func() interface{} { return &NoiseData{} }).
		AuditTime(100).
		Map(store)
	return stream
}

func DataID() []byte {
	return []byte { 0x34 }
}
