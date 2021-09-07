package main

import (
	"log"
	"os"

	y3 "github.com/yomorun/y3-codec-golang"
	"github.com/yomorun/yomo"
)

// Handler will handle data in Rx way
func handler(data []byte) (byte, []byte) {
	value, err := y3.ToFloat32(data)
	if err != nil {
		log.Printf("[stream-fn-db] y3.ToFloat32 err=%v", err)
		return 0x35, []byte{8}
	}
	log.Printf("[stream-fn-db] save `%v` to FaunaDB\n", value)
	return 0x35, data
}

func main() {
	sfn := yomo.NewStreamFunction(yomo.WithName("MockDB"), yomo.WithZipperAddr("localhost:9000"))
	defer sfn.Close()

	// 开始监听 dataID 为 0x30~0x33 的数据
	sfn.SetObserveDataID(0x34)

	// 设置要执行的函数
	sfn.SetHandler(handler)

	// 开始执行
	err := sfn.Connect()
	if err != nil {
		log.Printf("[stream-fn-db] connect err=%v", err)
		os.Exit(1)
	}
	select {}
}
