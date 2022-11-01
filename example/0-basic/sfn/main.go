package main

import (
	"encoding/json"
	"os"

	"github.com/yomorun/yomo"
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
		"Noise",
		yomo.WithZipperAddr(addr),
		yomo.WithObserveDataTags(0x33),
	)
	defer sfn.Close()

	// set handler
	sfn.SetHandler(handler)
	// start
	err := sfn.Connect()
	if err != nil {
		logger.Errorf("[sfn1] connect err=%v", err)
		os.Exit(1)
	}
	// set the error handler function when server error occurs
	sfn.SetErrorHandler(func(err error) {
		logger.Errorf("[sfn1] receive server error: %v", err)
		sfn.Close()
		os.Exit(1)
	})

	select {}
}

func handler(data []byte) (uint32, []byte) {
	var model noiseData
	err := json.Unmarshal(data, &model)
	if err != nil {
		logger.Errorf("[sfn] json.Marshal err=%v", err)
		os.Exit(-2)
	} else {
		logger.Printf(">> [sfn] got tag=0x33, data=%+v", model)
	}
	return 0x0, nil
}
