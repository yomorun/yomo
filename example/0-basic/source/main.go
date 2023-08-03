package main

import (
	"encoding/json"
	"math/rand"
	"os"
	"time"

	"github.com/yomorun/yomo"
	"golang.org/x/exp/slog"
)

// custom logger
var logger = slog.New(slog.NewTextHandler(os.Stdout, nil))

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
		addr,
		yomo.WithLogger(logger),
	)
	err := source.Connect()
	if err != nil {
		logger.Error("[source] ❌ Emit the data to YoMo-Zipper failure with err", "err", err)
		return
	}

	defer source.Close()

	// set the error handler function when server error occurs
	source.SetErrorHandler(func(err error) {
		logger.Error("[source] receive server error", "err", err)
		os.Exit(1)
	})

	// generate mock data and send it to YoMo-Zipper in every 100 ms.
	err = generateAndSendData(source)
	logger.Error("[source] >>>> ERR", "err", err)
	os.Exit(0)
}

func generateAndSendData(stream yomo.Source) error {
	i := 0
	for {
		// generate random data.
		data := noiseData{
			Noise: rand.New(rand.NewSource(time.Now().UnixNano())).Float32() * 200,
			Time:  time.Now().UnixNano() / int64(time.Millisecond),
			From:  "localhost",
		}

		sendingBuf, err := json.Marshal(&data)
		if err != nil {
			logger.Error("json.Marshal error", "err", err)
			os.Exit(-1)
		}

		// send data via QUIC stream.
		err = stream.Write(0x33, sendingBuf)
		i++
		if i > 6 {
			stream.Close()
			return nil
		}
		if err != nil {
			logger.Error("[source] ❌ Emit to YoMo-Zipper failure with err", "err", err, "data", data)
			time.Sleep(500 * time.Millisecond)
			continue

		} else {
			logger.Info("[source] ✅ Emit to YoMo-Zipper", "data", data)
		}

		time.Sleep(1000 * time.Millisecond)
	}
}
