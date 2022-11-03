package main

import (
	"os"
	"strconv"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/pkg/logger"
)

type noiseData struct {
	Noise float32 `json:"noise"` // Noise value
	Time  int64   `json:"time"`  // Timestamp (ms)
	From  string  `json:"from"`  // Source IP
}

func main() {
	addr := "localhost:9000"
	if v := os.Getenv("YOMO_ADDR"); v != "" {
		addr = v
	}
	sfn := yomo.NewStreamFunction(
		"sfn-1",
		yomo.WithZipperAddr(addr),
		yomo.WithObserveDataTags(0x33),
	)
	defer sfn.Close()

	// set handler
	sfn.SetHandler(handler)
	// start
	err := sfn.Connect()
	if err != nil {
		logger.Errorf("[sfn-1] connect err=%v", err)
		os.Exit(1)
	}
	// set the error handler function when server error occurs
	sfn.SetErrorHandler(func(err error) {
		logger.Errorf("[sfn-1] receive server error: %v", err)
		sfn.Close()
		os.Exit(1)
	})

	select {}
}

func handler(data []byte) (frame.Tag, []byte) {
	// got
	noise, err := strconv.ParseFloat(string(data), 10)
	if err != nil {
		logger.Errorf("[sfn-1] got err=%v", err)
		return 0x0, nil
	}
	// result
	result := int(noise)
	logger.Printf("[sfn-1] got: tag=0x33, data=%v, return: tag=0x34, data=%v", noise, result)

	return 0x34, []byte(strconv.Itoa(result))
}
