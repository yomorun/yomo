package main

import (
	"os"

	"golang.org/x/exp/slog"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/serverless"
)

func main() {
	sink := yomo.NewStreamFunction("sink", "localhost:9000")
	sink.SetObserveDataTags(0x61)
	sink.SetHandler(func(ctx serverless.Context) {
		slog.Info("[sink] receive data", "data", string(ctx.Data()))
	})
	err := sink.Connect()
	if err != nil {
		slog.Error("[sink] connect", "err", err)
		os.Exit(1)
	}
	defer sink.Close()
	sink.Wait()
}
