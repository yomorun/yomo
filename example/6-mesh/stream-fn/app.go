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
	"github.com/yomorun/yomo/serverless"
)

// NoiseDataKey represents the Tag of a Y3 encoded data packet
const NoiseDataKey = 0x10

// NoiseData represents the structure of data
type NoiseData struct {
	Noise float32 `json:"noise"`
	Time  int64   `json:"time"`
	From  string  `json:"from"`
}

var echo = func(_ context.Context, i interface{}) (interface{}, error) {
	value := i.(*NoiseData)
	rightNow := time.Now().UnixNano() / int64(time.Millisecond)
	fmt.Printf("%s %d > value: %f ⚡️=%dms\n", value.From, value.Time, value.Noise, rightNow-value.Time)
	value.Noise = value.Noise / 10
	return value, nil
}

func Handler(ctx serverless.Context) {
	data := ctx.Data()
	log.Printf("✅ [fn] receive <- %v", string(data))

	nd := &NoiseData{}
	_ = json.Unmarshal(data, nd)

	e, _ := echo(context.Background(), nd)

	r, _ := json.Marshal(e)
	ctx.Write(0x14, r)
}

func main() {
	addr := fmt.Sprintf("%s:%d", "localhost", getPort())
	sfn := yomo.NewStreamFunction(
		"Noise",
		addr,
	)
	sfn.SetObserveDataTags(DataTags()...)
	defer sfn.Close()

	// set handler
	sfn.SetHandler(Handler)

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

	sfn.Wait()
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
