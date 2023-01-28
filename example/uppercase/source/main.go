package main

import (
	"fmt"
	"log"
	"time"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/core/frame"
)

func main() {
	addr := "localhost:9000"

	source := yomo.NewSource(
		"source",
		yomo.WithZipperAddr(addr),
	)
	source.SetDataTag(0x33)
	if err := source.Connect(); err != nil {
		log.Fatalln(err)
	}
	defer source.Close()

	sink := yomo.NewStreamFunction(
		"sink",
		yomo.WithZipperAddr(addr),
		yomo.WithObserveDataTags(0x34),
	)
	sink.SetHandler(
		func(data []byte) (frame.Tag, []byte) {
			log.Printf("[recv] %s", string(data))
			return 0, nil
		},
	)
	if err := sink.Connect(); err != nil {
		log.Fatalln(err)
	}
	defer sink.Close()

	const HelloData = "Hello, YoMo!"
	for i := 0; ; i++ {
		data := fmt.Sprintf("[%d] %s", i, HelloData)
		source.Write([]byte(data))
		log.Printf("[send] %s", data)
		time.Sleep(1 * time.Second)
	}
}
