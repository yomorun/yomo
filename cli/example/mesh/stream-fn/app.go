package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	y3 "github.com/yomorun/y3-codec-golang"
	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/core/rx"
)

// NoiseDataKey represents the Tag of a Y3 encoded data packet
const NoiseDataKey = 0x10

// NoiseData represents the structure of data
type NoiseData struct {
	Noise float32 `y3:"0x11"`
	Time  int64   `y3:"0x12"`
	From  string  `y3:"0x13"`
}

var region = os.Getenv("REGION")

var printer = func(_ context.Context, i interface{}) (interface{}, error) {
	value := i.(NoiseData)
	rightNow := time.Now().UnixNano() / int64(time.Millisecond)
	fmt.Println(fmt.Sprintf("[%s] %d > value: %f ⚡️=%dms", value.From, value.Time, value.Noise, rightNow-value.Time))
	return fmt.Sprint(region, " ", value.Noise), nil
}

var callback = func(v []byte) (interface{}, error) {
	var mold NoiseData
	err := y3.ToObject(v, &mold)
	if err != nil {
		return nil, err
	}
	mold.Noise = mold.Noise / 10
	return mold, nil
}

// Handler will handle data in Rx way
func Handler(rxstream rx.Stream) rx.Stream {
	log.Println("Handler is running...")
	stream := rxstream.
		Subscribe(NoiseDataKey).
		OnObserve(callback).
		Debounce(50).
		Map(printer).
		Encode(0x14)

	return stream
}

func main() {
	cli, err := yomo.NewStreamFn(yomo.WithName("Noise")).Connect("localhost", getPort())
	if err != nil {
		log.Print("❌ Connect to YoMo-Zipper failure: ", err)
		return
	}

	defer cli.Close()
	cli.Pipe(Handler)
}

func getPort() int {
	port := 9000
	if os.Getenv("PORT") != "" && os.Getenv("PORT") != "9000" {
		port, _ = strconv.Atoi(os.Getenv("PORT"))
	}
	
	return port
}
