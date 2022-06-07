package main

import (
	"encoding/json"
	stdlog "log"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/core/log"
)

var logger = NewCustomLogger()

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
		yomo.WithLogger(logger),
		yomo.WithObserveDataTags(0x34, 0x0),
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
	source.SetReceiveHandler(func(tag byte, data []byte) {
		logger.Printf("[source] ♻️  receive backflow: tag=%#v, data=%v", tag, string(data))
	})

	// generate mock data and send it to YoMo-Zipper in every 100 ms.
	err = generateAndSendData(source)
	logger.Printf("[source] >>>> ERR >>>> %v", err)
	os.Exit(0)
}

func generateAndSendData(stream yomo.Source) error {
	var i = 0
	for {
		// generate random data.
		data := noiseData{
			Noise: rand.New(rand.NewSource(time.Now().UnixNano())).Float32() * 200,
			Time:  time.Now().UnixNano() / int64(time.Millisecond),
			From:  "localhost",
		}

		sendingBuf, err := json.Marshal(&data)
		if err != nil {
			logger.Errorf("json.Marshal err:%v", err)
			os.Exit(-1)
		}

		// send data via QUIC stream.
		_, err = stream.Write(sendingBuf)
		i++
		if i > 6 {
			stream.Close()
			return nil
		}
		if err != nil {
			logger.Errorf("[source] ❌ Emit %v to YoMo-Zipper failure with err: %v", data, err)
			time.Sleep(500 * time.Millisecond)
			continue

		} else {
			logger.Printf("[source] ✅ Emit %v to YoMo-Zipper", data)
		}

		time.Sleep(1000 * time.Millisecond)
	}
}

// custom logger

var _ = log.Logger(&CustomLogger{})

type CustomLogger struct {
	level log.Level
}

func NewCustomLogger() log.Logger {
	envLevel := strings.ToLower(os.Getenv("YOMO_LOG_LEVEL"))
	level := log.ErrorLevel
	switch envLevel {
	case "debug":
		level = log.DebugLevel
	case "info":
		level = log.InfoLevel
	case "warn":
		level = log.WarnLevel
	case "error":
		level = log.ErrorLevel
	}

	return &CustomLogger{
		level: level,
	}
}

func (c *CustomLogger) SetLevel(level log.Level) {
	c.level = level
}

func (c *CustomLogger) SetEncoding(encoding string) {
}

// Printf prints a formated message at LevelNo
func (c *CustomLogger) Printf(template string, args ...interface{}) {
	c.log(log.NoLevel, template, args...)
}

// Debugf logs a message at LevelDebug.
func (c *CustomLogger) Debugf(template string, args ...interface{}) {
	c.log(log.DebugLevel, template, args...)
}

// Infof logs a message at LevelInfo.
func (c *CustomLogger) Infof(template string, args ...interface{}) {
	c.log(log.InfoLevel, template, args...)
}

// Warnf logs a message at LevelWarn.
func (c *CustomLogger) Warnf(template string, args ...interface{}) {
	c.log(log.WarnLevel, template, args...)
}

// Errorf logs a message at LevelError.
func (c *CustomLogger) Errorf(template string, args ...interface{}) {
	c.log(log.ErrorLevel, template, args...)
}

// Output file path to write log message output to
func (c *CustomLogger) Output(file string) {
}

// ErrorOutput file path to write error message output to
func (c *CustomLogger) ErrorOutput(file string) {
}

func (c *CustomLogger) log(level log.Level, template string, args ...interface{}) {
	if c.level == log.Disabled {
		return
	}

	v := []interface{}{level}
	v = append(v, args...)
	if c.level == log.NoLevel {
		stdlog.Printf("%s "+template, v...)
		return
	}
	if level >= c.level {
		stdlog.Printf("%s "+template, v...)
	}
}
