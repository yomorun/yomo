package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/serverless"
)

type NoiseData struct {
	Noise float32 `json:"noise"`
	Time  int64   `json:"time"`
	From  string  `json:"from"`
}

// Handler will handle the raw data.
func Handler(ctx serverless.Context) {
	data := ctx.Data()
	var noise NoiseData
	err := json.Unmarshal(data, &noise)
	if err != nil {
		log.Printf(">> [sink] unmarshal data failed, err=%v", err)
	} else {
		log.Printf("%s >> [sink] save `%v` to FaunaDB\n", noise.From, noise.Noise)
	}
}

// DataTags observe tag list
func DataTags() []uint32 {
	return []uint32{0x10}
}

func main() {
	addr := fmt.Sprintf("%s:%d", "localhost", getPort())
	sfn := yomo.NewStreamFunction(
		"MockDB",
		addr,
	)
	sfn.SetObserveDataTags(DataTags()...)
	defer sfn.Close()

	// set handler
	sfn.SetHandler(yomo.AsyncHandleFunc(Handler))

	// set error handler
	sfn.SetErrorHandler(func(err error) {
		log.Printf("[MockDB] error handler: %v", err)
	})

	// start
	err := sfn.Connect()
	if err != nil {
		log.Print("❌ Connect to YoMo-Zipper failure: ", err)
		return
	}

	select {}
}

func getPort() int {
	port := 9000
	if os.Getenv("PORT") != "" && os.Getenv("PORT") != "9000" {
		port, _ = strconv.Atoi(os.Getenv("PORT"))
	}

	return port
}
