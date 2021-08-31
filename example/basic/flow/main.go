package main

import (
	"fmt"
	"os"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/logger"
)

func main() {
	sfn := yomo.NewStreamFunction(yomo.WithName("yomo-sfn"), yomo.WithZipperEndpoint("localhost:9000"))
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
	logger.Debugf("[flow] => go [tag=0x33] data=%# x, str: %s", data, data)
	return 0x34, []byte{byte(len(data))}
	// return 0x34, []byte{8}
}
