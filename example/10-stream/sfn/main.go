package main

import (
	"io"
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
		"sfn-stream",
		addr,
	)
	sfn.SetObserveDataTags(0x33)
	defer sfn.Close()

	// set handler
	sfn.SetHandler(handler)
	// start
	err := sfn.Connect()
	if err != nil {
		slog.Error("[sfn] connect", err)
		os.Exit(1)
	}
	// set the error handler function when server error occurs
	sfn.SetErrorHandler(func(err error) {
		slog.Error("[sfn] receive server error", "err", err)
	})

	select {}
}

func handler(ctx serverless.Context) {
	if ctx.Streamed() {
		handleStream(ctx)
		return
	}
	handleData(ctx)
}

func handleStream(ctx serverless.Context) {
	dataStream := ctx.Stream()
	if dataStream != nil {
		buf, err := io.ReadAll(dataStream)
		if err != nil {
			slog.Error("[sfn] failed to read all", "err", err)
			return
		}
		bufString := string(buf)
		l := len(buf)
		if l > 1000 {
			bufString = string(buf[l-1000:])
		}
		slog.Info("[sfn] read all", "len", l, "buf", bufString)
	} else {
		slog.Info("[sfn] dataStream is nil")
	}
}

func handleData(ctx serverless.Context) {
	slog.Info("[sfn] got", "data", ctx.Data())
}
