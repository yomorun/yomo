package main

import (
	"math/rand"
	"os"
	"strconv"
	"time"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/pkg/logger"
)

func main() {
	// connect to YoMo-Zipper.
	addr := "localhost:9000"
	if v := os.Getenv("YOMO_ADDR"); v != "" {
		addr = v
	}
	source := yomo.NewSource(
		"yomo-source",
		yomo.WithZipperAddr(addr),
		yomo.WithObserveDataTags(0x34, 0x35),
	)
	err := source.Connect()
	if err != nil {
		logger.Printf("[source] ❌ Emit the data to YoMo-Zipper failure with err: %v", err)
		return
	}

	defer source.Close()

	source.SetDataTag(0x33)
	// set the error handler function when server error occurs
	source.SetErrorHandler(func(err error) {
		logger.Printf("[source] receive server error: %v", err)
		os.Exit(1)
	})
	// set receive handler for the observe datatags
	source.SetReceiveHandler(func(tag frame.Tag, data []byte) {
		logger.Printf("[source] ♻️  receive backflow: tag=%#v, data=%s", tag, data)
	})

	// generate mock data and send it to YoMo-Zipper in every 100 ms.
	err = generateAndSendData(source)
	logger.Printf("[source] >>>> ERR >>>> %v", err)
	os.Exit(0)
}

func generateAndSendData(stream yomo.Source) error {
	for {
		// generate random data.
		noise := rand.New(rand.NewSource(time.Now().UnixNano())).Float64() * 200
		data := []byte(strconv.FormatFloat(noise, 'f', 2, 64))
		// send data via QUIC stream.
		_, err := stream.Write(data)
		if err != nil {
			logger.Errorf("[source] ❌ Emit %.2f to YoMo-Zipper failure with err: %v", noise, err)
			time.Sleep(500 * time.Millisecond)
			continue

		} else {
			logger.Printf("[source] ✅ Emit %.2f to YoMo-Zipper", noise)
		}

		time.Sleep(1000 * time.Millisecond)
	}
}
