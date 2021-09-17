package main

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/yomorun/yomo/cli/rx"
)

// NoiseData represents the structure of data
type NoiseData struct {
	Noise float32 `json:"noise"` // Noise value
	Time  int64   `json:"time"` // Timestamp (ms)
	From  string  `json:"from"` // Source IP
}

var printer = func(_ context.Context, i interface{}) (interface{}, error) {
	value := i.(*NoiseData)
	value.Noise = value.Noise / 10
	rightNow := time.Now().UnixNano() / int64(time.Millisecond)
	log.Printf(">> [flow] [%s] %d > value: %f ⚡️=%dms", value.From, value.Time, value.Noise, rightNow-value.Time)
	return value.Noise, nil
}

// Handler will handle data in Rx way
func Handler(rxstream rx.Stream) rx.Stream {
	log.Println("Handler is running...")
	stream := rxstream.
		Unmarshal(json.Unmarshal, func() interface{} { return &NoiseData{} }).
		Debounce(50).
		Map(printer).
		StdOut().
		Marshal(json.Marshal).
		PipeBackToZipper(0x34)

	return stream
}

func DataID() []byte {
	return []byte { 0x33 }
}
