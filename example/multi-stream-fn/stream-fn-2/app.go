package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"os"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/pkg/logger"
)

// ThresholdSingleValue is the threshold of a single value.
const ThresholdSingleValue = 16

// Print every value and alert for value greater than ThresholdSingleValue
var computePeek = func(_ context.Context, value float32) (float32, error) {
	fmt.Printf("✅ receive noise value: %f\n", value)

	// Compute peek value, if greater than ThresholdSingleValue, alert
	if value >= ThresholdSingleValue {
		fmt.Printf("❗ value: %f reaches the threshold %d! 𝚫=%f", value, ThresholdSingleValue, value-ThresholdSingleValue)
	}

	return value, nil
}

// main will observe data with SeqID=0x14, and tranform to SeqID=0x15 with Noise value
// to downstream sfn.
func main() {
	sfn := yomo.NewStreamFunction("Noise-2", yomo.WithZipperAddr("localhost:9000"))
	defer sfn.Close()

	sfn.SetObserveDataID(0x14)
	sfn.SetHandler(handler)

	err := sfn.Connect()
	if err != nil {
		logger.Errorf("[fn2] connect err=%v", err)
		os.Exit(1)
	}

	select {}
}

func handler(data []byte) (byte, []byte) {
	v := Float32frombytes(data)
	result, err := computePeek(context.Background(), v)
	if err != nil {
		logger.Errorf("[fn2] computePeek err=%v", err)
		return 0x0, nil
	}

	return 0x15, float32ToByte(result)
}

func Float32frombytes(bytes []byte) float32 {
	bits := binary.BigEndian.Uint32(bytes)
	return math.Float32frombits(bits)
}

func float32ToByte(f float32) []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, f)
	return buf.Bytes()
}
