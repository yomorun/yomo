package main

import (
	"encoding/json"
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/pkg/logger"
)

type noiseData struct {
	Noise float32 `json:"noise"` // Noise value
	Time  int64   `json:"time"`  // Timestamp (ms)
	From  string  `json:"from"`  // Source IP
}

func main() {
	// connect to YoMo-Zipper.
	addr := "localhost:9000"
	if v := os.Getenv("YOMO_ADDR"); v != "" {
		addr = v
	}
	source := yomo.NewSource(
		"yomo-source",
		yomo.WithZipperAddr(addr),
	)
	err := source.Connect()
	if err != nil {
		logger.Printf("[source] ❌ Emit the data to YoMo-Zipper failure with err: %v", err)
		return
	}

	defer source.Close()

	source.SetDataTag(0x33)

	// generate mock data and send it to YoMo-Zipper in every 100 ms.
	err = generateAndSendData(source)
	if err != nil {
		logger.Printf("[source] >>>> ERR >>>> %v", err)
		os.Exit(1)
	}
	select {}
}

// var codec = y3.NewCodec(0x10)

func generateAndSendData(stream yomo.Source) error {
	for {
		// generate random data.
		data := noiseData{
			Noise: rand.New(rand.NewSource(time.Now().UnixNano())).Float32() * 200,
			Time:  time.Now().UnixNano() / int64(time.Millisecond),
			From:  "localhost",
		}

		//// Encode data via Y3 codec https://github.com/yomorun/y3-codec.
		// sendingBuf, _ := codec.Marshal(data)
		sendingBuf, err := json.Marshal(&data)
		if err != nil {
			log.Fatalln(err)
			os.Exit(-1)
		}

		// send data via QUIC stream.
		_, err = stream.Write(sendingBuf)
		if err != nil {
			logger.Printf("[source] ❌ Emit %v to YoMo-Zipper failure with err: %v", data, err)
			time.Sleep(500 * time.Millisecond)
			continue

		} else {
			logger.Printf("[source] ✅ Emit %v to YoMo-Zipper", data)
		}

		time.Sleep(500 * time.Millisecond)
	}
}
