package main

import (
	"encoding/json"
	"os"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/core/frame"
	"golang.org/x/exp/slog"
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
		slog.Error("[sfn1] connect", err)
		os.Exit(1)
	}
	// set the error handler function when server error occurs
	sfn.SetErrorHandler(func(err error) {
		slog.Error("[sfn1] receive server error", err)
		sfn.Close()
		os.Exit(1)
	})

	select {}
}

func handler(data []byte) (frame.Tag, []byte) {
	var model noiseData
	err := json.Unmarshal(data, &model)
	if err != nil {
		slog.Error("[sfn] json.Marshal error", err)
		os.Exit(-2)
	} else {
		slog.Info("[sfn]", "got", 0x33, "data", model)
	}
	return 0x0, nil
}
