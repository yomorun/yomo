package main

import (
	"log"
	"os"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/serverless"
)

func main() {
	sfn := yomo.NewStreamFunction(
		"echo-sfn",
		"localhost:9002",
		yomo.WithSfnCredential("token:z2"),
	)
	sfn.SetObserveDataTags(0x33)
	defer sfn.Close()

	// set handler
	sfn.SetHandler(handler)

	// start
	err := sfn.Connect()
	if err != nil {
		log.Fatalf("[sfn] connect err=%v", err)
		os.Exit(1)
	}

	sfn.Wait()
}

func handler(ctx serverless.Context) {
	val := string(ctx.Data())
	log.Printf(">> [sfn] got tag=0x33, data=%s", val)
}
