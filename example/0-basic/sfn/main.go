package main

import (
	"encoding/json"
	"os"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/serverless"
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
		addr,
	)
	sfn.SetObserveDataTags(0x33)
	defer sfn.Close()

	// set handler
	sfn.SetHandler(handler)
	// start
	err := sfn.Connect()
	if err != nil {
		slog.Error("[sfn] connect", "err", err)
		os.Exit(1)
	}
	// set the error handler function when server error occurs
	sfn.SetErrorHandler(func(err error) {
		slog.Error("[sfn] receive server error", "err", err)
	})

	sfn.Wait()
}

func handler(ctx serverless.Context) {
	var model noiseData
	err := json.Unmarshal(ctx.Data(), &model)
	if err != nil {
		slog.Error("[sfn] json.Marshal error", "err", err)
		os.Exit(-2)
	} else {
		slog.Info("[sfn]", "got", 0x33, "data", model)
	}
}
