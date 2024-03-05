package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"log"
	"os"
	"time"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/serverless"
)

// NoiseData represents the structure of data
type noiseData struct {
	Noise float32 `json:"noise"` // Noise value
	Time  int64   `json:"time"`  // Timestamp (ms)
	From  string  `json:"from"`  // Source IP
}

// main will observe data with SeqID=0x10, and tranform to SeqID=0x14 with Noise value
// to downstream sfn.
func main() {
	// sfn
	sfn := yomo.NewStreamFunction(
		"Noise-1",
		"localhost:9000",
	)
	sfn.SetObserveDataTags(0x10)
	defer sfn.Close()

	sfn.SetHandler(handler)

	err := sfn.Connect()
	if err != nil {
		log.Printf("[fn1] connect err=%v", err)
		os.Exit(1)
	}

	sfn.Wait()
}

func handler(ctx serverless.Context) {
	data := ctx.Data()
	var mold noiseData
	err := json.Unmarshal(data, &mold)
	if err != nil {
		log.Printf("[fn1] y3.ToObject err=%v", err)
		return
	}
	mold.Noise = mold.Noise / 10

	// Print every value and return noise value to downstream.
	result, err := printExtract(context.Background(), &mold)
	if err != nil {
		log.Printf("[fn1] to downstream err=%v", err)
		return
	}

	// transfer result to downstream
	ctx.Write(0x14, float32ToByte(result))
}

// Print every value and return noise value to downstream.
var printExtract = func(_ context.Context, value *noiseData) (float32, error) {
	rightNow := time.Now().UnixNano() / int64(time.Millisecond)
	log.Printf("✅ [%s] %d > value: %f ⚡️=%dms", value.From, value.Time, value.Noise, rightNow-value.Time)

	return value.Noise, nil
}

func float32ToByte(f float32) []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, f)
	return buf.Bytes()
}
