package main

import (
	"encoding/json"
	"os"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/logger"
)

type noiseData struct {
	Noise float32 `json:"noise"` // Noise value
	Time  int64   `json:"time"`  // Timestamp (ms)
	From  string  `json:"from"`  // Source IP
}

func main() {
	sfn := yomo.NewStreamFunction("Noise", yomo.WithZipperAddr("localhost:9000"))
	defer sfn.Close()

	// 开始监听 dataID 为 0x33 的数据
	sfn.SetObserveDataID(0x33)

	// 设置要执行的函数
	sfn.SetHandler(handler)

	// 开始执行
	err := sfn.Connect()
	if err != nil {
		logger.Errorf("[flow] connect err=%v", err)
		os.Exit(1)
	}

	select {}
}

func handler(data []byte) (byte, []byte) {
	// var model NoiseData
	// err := y3.ToObject(data, &model)
	// if err != nil {
	// 	logger.Errorf("[flow] y3.ToObject err=%v", err)
	// 	return 0x0, nil
	// }
	// logger.Printf("[flow] got tag=0x33, data=%v", model)
	// return 0x0, nil

	var model noiseData
	err := json.Unmarshal(data, &model)
	if err != nil {
		logger.Errorf("[flow] json.Marshal err=%v", err)
		os.Exit(-2)
	} else {
		logger.Printf(">>>>>>>>>>>>>>> [flow] got tag=0x33, data=%v", model)
	}
	return 0x0, nil
}
