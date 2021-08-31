package main

import (
	"fmt"
	"os"

	"github.com/yomorun/y3-codec-golang"
	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/logger"
)

// NoiseData represents the structure of data
type NoiseData struct {
	Noise float32 `y3:"0x11"`
	Time  int64   `y3:"0x12"`
	From  string  `y3:"0x13"`
}

func main() {
	sfn := yomo.NewStreamFunction(yomo.WithName("Noise"), yomo.WithZipperEndpoint("localhost:9000"))
	defer sfn.Close()

	// 开始监听 dataID 为 0x30~0x33 的数据
	sfn.SetObserveDataID(0x33)

	// 设置要执行的函数
	sfn.SetHandler(handler)

	// 开始执行
	err := sfn.Connect()
	if err != nil {
		logger.Errorf("[flow] connect err=%v", err)
		os.Exit(1)
	}

	// time.Sleep(30 * time.Second)
	fmt.Scanf("[flow] Press to stop")
}

func handler(data []byte) (byte, []byte) {
	var model NoiseData
	err := y3.ToObject(data, &model)
	if err != nil {
		logger.Errorf("[flow] y3.ToObject err=%v", err)
		return 0x34, []byte{8}
	}
	logger.Debugf("[flow] => go [tag=0x33] data=%# x, model: %#v", data, model)
	return 0x34, data
	// return 0x34, []byte{8}
}
