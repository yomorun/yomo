package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/yomorun/yomo"
	"golang.org/x/exp/slog"
)

func main() {
	// connect to YoMo-Zipper.
	addr := "localhost:9000"
	if v := os.Getenv("YOMO_ADDR"); v != "" {
		addr = v
	}
	source := yomo.NewSource(
		"yomo-source",
		addr,
	)
	err := source.Connect()
	if err != nil {
		slog.Info("[source] ❌ Emit the data to YoMo-Zipper failure with err", "err", err)
		return
	}

	defer source.Close()

	// set the error handler function when server error occurs
	source.SetErrorHandler(func(err error) {
		slog.Error("[source] receive server error", "err", err)
	})

	streamed := true
	if v := os.Getenv("YOMO_STREAMED"); v != "" {
		s, err := strconv.ParseBool(v)
		if err == nil {
			streamed = s
		}
	}
	slog.Info(fmt.Sprintf("[source] use stream: %v", streamed))

	if streamed {
		err = pipeStream(source)
	} else {
		err = write(source)
	}
	slog.Info("[source] err: ", "err", err)
	if err != nil {
		slog.Error("[source] >>>> ERR", "err", err)
		// os.Exit(0)
	}
	select {}
}

func pipeStream(source yomo.Source) error {
	for i := 0; ; i++ {
		// read data from file.
		d := i % 2
		file := fmt.Sprintf("%d.dat", d)
		slog.Info(fmt.Sprintf("[source] #%d. pipe stream to YoMo-Zipper", i))
		pipeFile(source, file)
		go pipeFile(source, "0.dat")
		go pipeFile(source, "1.dat")
		time.Sleep(time.Second * 1)
	}
}

func pipeFile(source yomo.Source, file string) error {
	reader, err := os.Open(file)
	if err != nil {
		slog.Error("[source] ❌ Read file failure with err", "err", err)
		return err
	}
	// defer reader.Close()
	// send data to YoMo-Zipper.
	slog.Info("[source] pipe stream to YoMo-Zipper", "stream", file)
	err = source.Pipe(0x33, reader)
	if err != nil {
		slog.Error("[source] ❌ Emit to YoMo-Zipper failure with err", "err", err)
		return err
	}
	return nil
}

func write(source yomo.Source) error {
	for {
		time.Sleep(1000 * time.Millisecond)
		n := time.Now().UnixMilli()
		data := strconv.FormatInt(n, 10)
		slog.Info("[source] write data to YoMo-Zipper", "data", data)
		if err := source.Write(0x33, []byte(data)); err != nil {
			slog.Error("[source] ❌ Emit to YoMo-Zipper failure with err", "err", err)
			return err
		}
	}
}
