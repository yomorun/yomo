package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"log"
	"math"
	"os"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/serverless"
)

// ThresholdSingleValue is the threshold of a single value.
const ThresholdSingleValue = 16

// Print every value and alert for value greater than ThresholdSingleValue
var computePeek = func(_ context.Context, value float32) (float32, error) {
	log.Printf("✅ receive noise value: %f\n", value)

	// Compute peek value, if greater than ThresholdSingleValue, alert
	if value >= ThresholdSingleValue {
		log.Printf("❗ value: %f reaches the threshold %d! 𝚫=%f", value, ThresholdSingleValue, value-ThresholdSingleValue)
	}

	return value, nil
}

// main will observe data with SeqID=0x14, and tranform to SeqID=0x15 with Noise value
// to downstream sfn.
func main() {
	// sfn
	sfn := yomo.NewStreamFunction(
		"Noise-2",
		"localhost:9000",
	)
	sfn.SetObserveDataTags(0x14)
	defer sfn.Close()

	sfn.SetHandler(handler)

	err := sfn.Connect()
	if err != nil {
		log.Printf("[fn2] connect err=%v", err)
		os.Exit(1)
	}

	sfn.Wait()
}

func handler(ctx serverless.Context) {
	data := ctx.Data()
	v := Float32frombytes(data)
	result, err := computePeek(context.Background(), v)
	if err != nil {
		log.Printf("[fn2] computePeek err=%v", err)
		return
	}

	ctx.Write(0x15, float32ToByte(result))
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
