package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"os"
	"time"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/pkg/logger"
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
	sfn := yomo.NewStreamFunction(
		"Noise-1",
		yomo.WithZipperAddr("localhost:9000"),
		yomo.WithObserveDataTags(0x10),
	)
	defer sfn.Close()

	sfn.SetHandler(handler)

	err := sfn.Connect()
	if err != nil {
		logger.Errorf("[fn1] connect err=%v", err)
		os.Exit(1)
	}

	select {}
}

func handler(data []byte) (uint32, []byte) {
	var mold noiseData
	err := json.Unmarshal(data, &mold)
	if err != nil {
		logger.Errorf("[fn1] y3.ToObject err=%v", err)
		return 0x0, nil
	}
	mold.Noise = mold.Noise / 10

	// Print every value and return noise value to downstream.
	result, err := printExtract(context.Background(), &mold)
	if err != nil {
		logger.Errorf("[fn1] to downstream err=%v", err)
		return 0x0, nil
	}

	// transfer result to downstream
	return 0x14, float32ToByte(result)
}

// Print every value and return noise value to downstream.
var printExtract = func(_ context.Context, value *noiseData) (float32, error) {
	rightNow := time.Now().UnixNano() / int64(time.Millisecond)
	logger.Printf("✅ [%s] %d > value: %f ⚡️=%dms", value.From, value.Time, value.Noise, rightNow-value.Time)

	return value.Noise, nil
}

func float32ToByte(f float32) []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, f)
	return buf.Bytes()
}
