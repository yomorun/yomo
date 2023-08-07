package main

import (
	"fmt"
	"log"
	"time"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/serverless"
)

func main() {
	addr := "localhost:9000"

	source := yomo.NewSource(
		"source",
		addr,
	)
	if err := source.Connect(); err != nil {
		log.Fatalln(err)
	}
	defer source.Close()

	sink := yomo.NewStreamFunction(
		"sink",
		addr,
	)
	sink.SetObserveDataTags(0x34)
	sink.SetHandler(
		yomo.AsyncHandleFunc(func(ctx serverless.Context) {
			data := ctx.Data()
			log.Printf("[recv] %s", string(data))
		}),
	)
	if err := sink.Connect(); err != nil {
		log.Fatalln(err)
	}
	defer sink.Close()

	const HelloData = "Hello, YoMo!"
	for i := 0; ; i++ {
		data := fmt.Sprintf("[%d] %s", i, HelloData)
		source.Write(0x33, []byte(data))
		log.Printf("[send] %s", data)
		time.Sleep(1 * time.Second)
	}
}
