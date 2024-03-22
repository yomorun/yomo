package main

import (
	"log/slog"
	"os"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/serverless"
)

var instanceID = "bob"

func main() {
	if v := os.Getenv("USERID"); v != "" {
		instanceID = v
	}
	sfn := yomo.NewStreamFunction(
		"sink",
		"localhost:9000",
	)
	sfn.SetObserveDataTags(0x33)
	sfn.SetWantedTarget(instanceID)

	// set handler
	sfn.SetHandler(handler)
	// start
	err := sfn.Connect()
	if err != nil {
		slog.Error("[sfn] connect", "err", err)
		os.Exit(1)
	}
	defer sfn.Close()

	// set the error handler function when server error occurs
	sfn.SetErrorHandler(func(err error) {
		slog.Error("[sfn] receive server error", "err", err)
	})

	sfn.Wait()
}

func handler(ctx serverless.Context) {
	data := string(ctx.Data())
	slog.Info("Received", "uid", instanceID, "tag", ctx.Tag(), "data", data)
}
