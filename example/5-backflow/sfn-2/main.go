package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/core/frame"
)

func main() {
	addr := "localhost:9000"
	if v := os.Getenv("YOMO_ADDR"); v != "" {
		addr = v
	}
	sfn := yomo.NewStreamFunction(
		"sfn-2",
		yomo.WithZipperAddr(addr),
		yomo.WithObserveDataTags(0x34),
	)
	defer sfn.Close()

	// set handler
	sfn.SetHandler(handler)
	// start
	err := sfn.Connect()
	if err != nil {
		fmt.Printf("[sfn-2] connect err=%v", err)
		os.Exit(1)
	}
	// set the error handler function when server error occurs
	sfn.SetErrorHandler(func(err error) {
		fmt.Printf("[sfn-2] receive server error: %v", err)
		sfn.Close()
		os.Exit(1)
	})

	select {}
}

func handler(data []byte) (frame.Tag, []byte) {
	// got
	noise, err := strconv.Atoi(string(data))
	if err != nil {
		fmt.Printf("[sfn-2] got err=%v", err)
		return 0x0, nil
	}
	// result
	result := noise * 10
	fmt.Printf("[sfn-2] got: tag=0x34, data=%v, return: tag=0x35, data=%v", noise, result)

	return 0x35, []byte(strconv.Itoa(result))
}
