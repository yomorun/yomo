package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/serverless"
)

var i = 0
var target = "bob"

func main() {
	sfn := yomo.NewStreamFunction(
		"fn1",
		"localhost:9000",
	)
	defer sfn.Close()

	sfn.SetCronHandler("@every 1s", func(ctx serverless.CronContext) {
		if i%2 == 0 {
			target = "alice"
		} else {
			target = "bob"
		}
		// ctx.Write(0x33, []byte("message from cron sfn"))
		ctx.WriteWithTarget(0x33, []byte(fmt.Sprintf("message from cron sfn %d", i)), target)
		i++
	})
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
