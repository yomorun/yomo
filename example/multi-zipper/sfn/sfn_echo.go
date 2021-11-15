package main

import (
	"log"
	"os"

	"github.com/yomorun/yomo"
)

func main() {
	sfn := yomo.NewStreamFunction("echo-sfn", yomo.WithZipperAddr("localhost:9002"))
	defer sfn.Close()

	// set only monitoring data which tag=0x33
	sfn.SetObserveDataTag(0x33)

	// set handler
	sfn.SetHandler(handler)

	// start
	err := sfn.Connect()
	if err != nil {
		log.Fatalf("[flow] connect err=%v", err)
		os.Exit(1)
	}

	select {}
}

func handler(data []byte) (byte, []byte) {
	val := string(data)
	log.Printf(">> [flow] got tag=0x33, data=%s", val)
	return 0x0, nil
}
