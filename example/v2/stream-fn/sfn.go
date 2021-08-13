package main

import (
	"context"
	"fmt"
	"log"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/core/rx"
)

func main() {
	cli, err := yomo.NewStreamFn(yomo.WithName("sfn-1")).Connect("localhost", 9000)
	if err != nil {
		log.Print("❌ Connect to YoMo-Zipper failure: ", err)
		return
	}

	defer cli.Close()
	cli.Pipe(Handler)
}

// Handler will handle data in Rx way
func Handler(rxstream rx.Stream) rx.Stream {
	stream := rxstream.RawBytes().Map(m)

	return stream
}

var m = func(_ context.Context, v interface{}) (interface{}, error) {
	fmt.Printf("receive: %s\n", v)
	return v, nil
}
