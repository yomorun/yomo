package main

import (
	"log"
	"os"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/core/frame"
)

func main() {
	sfn := yomo.NewStreamFunction(
		"echo-sfn",
		yomo.WithZipperAddr("localhost:9002"),
		yomo.WithObserveDataTags(0x33),
		yomo.WithCredential("token:z2"),
	)
	defer sfn.Close()

	// set handler
	sfn.SetHandler(handler)

	// start
	err := sfn.Connect()
	if err != nil {
		log.Fatalf("[sfn] connect err=%v", err)
		os.Exit(1)
	}

	select {}
}

func handler(data []byte) (frame.Tag, []byte) {
	val := string(data)
	log.Printf(">> [sfn] got tag=0x33, data=%s", val)
	return 0x0, nil
}
