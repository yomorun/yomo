package main

import (
	"os"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/logger"
)

func main() {
	sfn := yomo.NewStreamFunction("echo-sfn", yomo.WithZipperAddr("localhost:9002"))
	defer sfn.Close()

	// set only monitoring data which tag=0x33
	sfn.SetObserveDataID(0x33)

	// set handler
	sfn.SetHandler(handler)

	// start
	err := sfn.Connect()
	if err != nil {
		logger.Errorf("[flow] connect err=%v", err)
		os.Exit(1)
	}

	select {}
}

func handler(data []byte) (byte, []byte) {
	val := string(data)
	logger.Printf(">> [flow] got tag=0x33, data=%s", val)
	return 0x0, nil
}
