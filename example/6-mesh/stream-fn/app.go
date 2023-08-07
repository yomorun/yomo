package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/rx"
)

// NoiseDataKey represents the Tag of a Y3 encoded data packet
const NoiseDataKey = 0x10

// NoiseData represents the structure of data
type NoiseData struct {
	Noise float32 `json:"noise"`
	Time  int64   `json:"time"`
	From  string  `json:"from"`
}

var region = os.Getenv("REGION")

var echo = func(_ context.Context, i interface{}) (interface{}, error) {
	value := i.(*NoiseData)
	rightNow := time.Now().UnixNano() / int64(time.Millisecond)
	fmt.Println(fmt.Sprintf("%s %d > value: %f ⚡️=%dms", value.From, value.Time, value.Noise, rightNow-value.Time))
	value.Noise = value.Noise / 10
	return value, nil
}

// Handler will handle data in Rx way
func Handler(rxstream rx.Stream) rx.Stream {
	log.Println("Handler is running...")
	stream := rxstream.
		Unmarshal(json.Unmarshal, func() interface{} { return &NoiseData{} }).
		Debounce(50).
		Map(echo).
		Marshal(json.Marshal).
		PipeBackToZipper(0x14)

	return stream
}

func main() {
	addr := fmt.Sprintf("%s:%d", "localhost", getPort())
	sfn := yomo.NewStreamFunction(
		"Noise",
		addr,
	)
	sfn.SetObserveDataTags(DataTags()...)
	defer sfn.Close()

	// create a Rx runtime.
	rt := rx.NewRuntime(sfn)

	// set handler
	sfn.SetHandler(yomo.AsyncHandleFunc(rt.RawByteHandler))

	// set error handler
	sfn.SetErrorHandler(func(err error) {
		log.Printf("[Noise] error handler: %v", err)
	})

	// start
	err := sfn.Connect()
	if err != nil {
		log.Print("❌ Connect to YoMo-Zipper failure: ", err)
		return
	}

	// pipe rx stream and rx handler.
	rt.Pipe(Handler)

	select {}
}

// DataTags observe tag list
func DataTags() []uint32 {
	return []uint32{0x10}
}

func getPort() int {
	port := 9000
	if os.Getenv("PORT") != "" && os.Getenv("PORT") != "9000" {
		port, _ = strconv.Atoi(os.Getenv("PORT"))
	}

	return port
}
