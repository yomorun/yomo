package main

import (
	"log"
	"os"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/pkg/trace"
	"github.com/yomorun/yomo/serverless"
)

func main() {
	// trace
	tp := trace.NewTracerProvider("yomo-sfn")
	// sfn
	sfn := yomo.NewStreamFunction(
		"Noise-4",
		"localhost:9000",
		yomo.WithSfnTracerProvider(tp),
	)
	sfn.SetObserveDataTags(0x10)
	defer sfn.Close()

	sfn.SetHandler(handler)

	err := sfn.Connect()
	if err != nil {
		log.Printf("[fn3] connect err=%v", err)
		os.Exit(1)
	}

	sfn.Wait()
}

func handler(ctx serverless.Context) {
	data := ctx.Data()
	log.Printf("✅ [fn4] receive <- %v", string(data))
}
