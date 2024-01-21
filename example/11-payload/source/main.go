package main

import (
	"fmt"
	"os"
	"time"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/core/payload"
	"github.com/yomorun/yomo/serverless"
)

func main() {
	addr := "localhost:9000"
	if v := os.Getenv("YOMO_ADDR"); v != "" {
		addr = v
	}
	source := yomo.NewSource("sender", addr)
	defer source.Close()

	err := source.Connect()
	if err != nil {
		fmt.Println("[source] ❌ Emit the data to YoMo-Zipper failure with err", err)
		return
	}

	sfn := yomo.NewStreamFunction("receiver", addr)
	sfn.SetObserveDataTags(0x02)
	sfn.SetHandler(func(ctx serverless.Context) {
		fmt.Printf("[sfn] Receive data: %s, tid: %s\n", string(ctx.Data()), ctx.TID())
	})
	sfn.SetWantedTarget("the-receiver-id")
	defer sfn.Close()

	err = sfn.Connect()
	if err != nil {
		fmt.Println("[sfn] ❌ Emit the data to YoMo-Zipper failure with err", err)
		return
	}

	i := 0
	for {
		var (
			paylaod = []byte(fmt.Sprintf("hello-%d", i))
			tid     = fmt.Sprintf("tid-%d", i)
			target  = "the-handler-id"
		)
		payload := payload.New(paylaod).WithTID(tid).WithTarget(target)
		err := source.WritePayload(0x01, payload)
		fmt.Println(err, payload)
		time.Sleep(time.Second)
		i++
	}
}
