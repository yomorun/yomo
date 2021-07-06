package main

import (
	"context"
	"fmt"
	"log"

	y3 "github.com/yomorun/y3-codec-golang"
	"github.com/yomorun/yomo/outconn"
	"github.com/yomorun/yomo/rx"
)

var store = func(_ context.Context, i interface{}) (interface{}, error) {
	value := i.(float32)
	fmt.Printf("save `%v` to FaunaDB\n", value)
	return value, nil
}

var callback = func(v []byte) (interface{}, error) {
	return y3.ToFloat32(v)
}

// Handler will handle data in Rx way
func Handler(rxstream rx.Stream) rx.Stream {
	stream := rxstream.
		Subscribe(0x11).
		OnObserve(callback).
		Map(store)
	return stream
}

func main() {
	cli, err := outconn.NewClient("MockDB").Connect("localhost", 9000)
	if err != nil {
		log.Print("❌ Connect to yomo-server failure: ", err)
		return
	}

	defer cli.Close()
	cli.Run(Handler)
}
