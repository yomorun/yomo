package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"

	y3 "github.com/yomorun/y3-codec-golang"
	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/core/rx"
)

var store = func(_ context.Context, i interface{}) (interface{}, error) {
	value := i.(string)
	fmt.Printf("save `%v` to FaunaDB\n", value)
	return value, nil
}

var callback = func(v []byte) (interface{}, error) {
	return y3.ToUTF8String(v)
}

// Handler will handle data in Rx way
func Handler(rxstream rx.Stream) rx.Stream {
	stream := rxstream.
		Subscribe(0x14).
		OnObserve(callback).
		AuditTime(100).
		Map(store)
	return stream
}

func main() {
	cli, err := yomo.NewStreamFn(yomo.WithName("MockDB")).Connect("localhost", getPort())
	if err != nil {
		log.Print("❌ Connect to YoMo-Zipper failure: ", err)
		return
	}

	defer cli.Close()
	cli.Pipe(Handler)
}

func getPort() int {
	port := 9000
	if os.Getenv("PORT") != "" && os.Getenv("PORT") != "9000" {
		port, _ = strconv.Atoi(os.Getenv("PORT"))
	}
	
	return port
}
