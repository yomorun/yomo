package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"math"
	"os"
	"time"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/pkg/logger"
)

// NoiseDataKey represents the Tag of a Y3 encoded data packet.
const NoiseDataKey = 0x10

// NoiseData represents the structure of data
type NoiseData struct {
	Noise float32 `json:"noise"`
	Time  int64   `json:"time"`
	From  string  `json:"from"`
}

// Print every value and return noise value to downstream.
var print = func(_ context.Context, value *NoiseData) (float32, error) {
	rightNow := time.Now().UnixNano() / int64(time.Millisecond)
	// fmt.Println(fmt.Sprintf("[%s] %d > value: %f ⚡️=%dms", value.From, value.Time, value.Noise, rightNow-value.Time))
	logger.Printf("[%s] %d > value: %f ⚡️=%dms", value.From, value.Time, value.Noise, rightNow-value.Time)

	return value.Noise, nil
}

func main() {
	sfn := yomo.NewStreamFunction("Noise", yomo.WithZipperAddr("localhost:9000"))
	defer sfn.Close()

	sfn.SetObserveDataTag(NoiseDataKey)
	sfn.SetHandler(handler)

	err := sfn.Connect()
	if err != nil {
		logger.Errorf("[fn1] connect err=%v", err)
		os.Exit(1)
	}

	select {}
}

func handler(data []byte) (byte, []byte) {
	var mold NoiseData
	err := json.Unmarshal(data, &mold)
	if err != nil {
		logger.Errorf("[fn1] json.Unmarshal err=%v", err)
		return 0x0, nil
	}
	mold.Noise = mold.Noise / 10
	// Print every value and return noise value to downstream.
	result, err := print(context.Background(), &mold)
	if err != nil {
		logger.Errorf("[fn1] to downstream err=%v", err)
		return 0x0, nil
	}

	return 0x14, Float32ToBytes(result)
}

func Float32frombytes(bytes []byte) float32 {
	bits := binary.BigEndian.Uint32(bytes)
	return math.Float32frombits(bits)
}

func Float32ToBytes(f float32) []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, f)
	return buf.Bytes()
}
