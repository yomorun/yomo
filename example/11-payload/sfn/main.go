package main

import (
	"fmt"
	"os"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/serverless"
)

func main() {
	addr := "localhost:9000"
	if v := os.Getenv("YOMO_ADDR"); v != "" {
		addr = v
	}

	sfn := yomo.NewStreamFunction("yomo-handler", addr)
	sfn.SetWantedTarget("the-receiver-id")
	sfn.SetObserveDataTags(0x01)
	sfn.SetHandler(func(ctx serverless.Context) {
		fmt.Printf("[sfn] Receive data: %s, tid: %s\n", string(ctx.Data()), ctx.TID())
		ctx.Write(0x02, ctx.Data())

	})
	sfn.SetWantedTarget("the-handler-id")
	defer sfn.Close()

	err := sfn.Connect()
	if err != nil {
		fmt.Println("[sfn] ❌ Emit the data to YoMo-Zipper failure with err", err)
		return
	}

	sfn.Wait()
}
