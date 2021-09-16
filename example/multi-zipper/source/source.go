package main

import (
	// "encoding/json"
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/yomorun/yomo"
)

import (
	"bytes"
	"encoding/binary"
	"math"
)

func Float32ToByte(f float32) []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, f)
	return buf.Bytes()
}

func Float32fromBytes(bytes []byte) float32 {
	bits := binary.BigEndian.Uint32(bytes)
	return math.Float32frombits(bits)
}

// type noiseData struct {
// 	Decibel float32 `json:"noise"` // Noise value
// 	Time    int64   `json:"time"`  // Timestamp (ms)
// 	From    string  `json:"from"`  // Source IP
// }

func main() {
	// connect to YoMo-Zipper.
	source := yomo.NewSource("yomo-source", yomo.WithZipperAddr("localhost:9001"))
	err := source.Connect()
	if err != nil {
		log.Printf("[source] ❌ Emit the data to YoMo-Zipper failure with err: %v", err)
		return
	}
	defer source.Close()

	source.SetDataTag(0x33)

	// generate mock data and send it to YoMo-Zipper.
	err = generateAndSendData(source)
	if err != nil {
		log.Printf("[source] >>>> ERR >>>> %v", err)
		os.Exit(1)
	}
	select {}
}

func generateAndSendData(stream yomo.Source) error {
	for {
		// generate random data.
		// data := noiseData{
		// 	Decibel: rand.New(rand.NewSource(time.Now().UnixNano())).Float32() * 200,
		// 	Time:    time.Now().UnixNano() / int64(time.Millisecond),
		// 	From:    "localhost",
		// }

		data := rand.New(rand.NewSource(time.Now().UnixNano())).Float32() * 200

		sendingBuf := Float32ToByte(data)

		// send data via QUIC stream.
		_, err := stream.Write(sendingBuf)
		if err != nil {
			log.Printf("[source] ❌ Emit %v to YoMo-Zipper failure with err: %v", data, err)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		log.Printf("[source] ✅ Emit %v to YoMo-Zipper", data)
		time.Sleep(2500 * time.Millisecond)
	}
}
