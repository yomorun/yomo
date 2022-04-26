package main

import (
	"os"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/pkg/logger"
)

func main() {
	addr := "localhost:9000"
	if v := os.Getenv("YOMO_ADDR"); v != "" {
		addr = v
	}
	sfn := yomo.NewStreamFunction(
		"Noise2",
		yomo.WithZipperAddr(addr),
		yomo.WithObserveDataTags(0x34),
	)
	defer sfn.Close()

	// set handler
	sfn.SetHandler(handler)
	// start
	err := sfn.Connect()
	if err != nil {
		logger.Errorf("[sfn-2] connect err=%v", err)
		os.Exit(1)
	}
	// set the error handler function when server error occurs
	sfn.SetErrorHandler(func(err error) {
		logger.Errorf("[sfn-2] receive server error: %v", err)
		sfn.Close()
		os.Exit(1)
	})

	select {}
}

func handler(data []byte) (byte, []byte) {
	logger.Printf(">> [sfn-2] got tag=0x34, data=%s", string(data))
	return 0x35, []byte("sfn-2 处理结果")
}
