package main

import (
	"log"
	"os"

	"github.com/yomorun/y3-codec-golang"
	"github.com/yomorun/yomo"
)

// NoiseDataKey represents the Tag of a Y3 encoded data packet
const NoiseDataKey = 0x10

// NoiseData represents the structure of data
type NoiseData struct {
	Noise float32 `y3:"0x11"`
	Time  int64   `y3:"0x12"`
	From  string  `y3:"0x13"`
}

// var printer = func(_ context.Context, i interface{}) (interface{}, error) {
// 	value := i.(NoiseData)
// 	rightNow := time.Now().UnixNano() / int64(time.Millisecond)
// 	fmt.Println(fmt.Sprintf("[%s] %d > value: %f ⚡️=%dms", value.From, value.Time, value.Noise, rightNow-value.Time))
// 	return value.Noise, nil
// }

func toObject(v []byte) (*NoiseData, error) {
	var mold NoiseData
	err := y3.ToObject(v, &mold)
	if err != nil {
		return nil, err
	}
	mold.Noise = mold.Noise * 10
	return &mold, nil
}

// Handler will handle data in Rx way
func handler(data []byte) (byte, []byte) {
	model, err := toObject(data)
	if err != nil {
		log.Printf("[stream-fn] y3.ToObject err=%v", err)
		return 0x34, []byte{8}
	}
	log.Printf("[stream-fn] => got data: %#v", model)

	p := y3.NewPrimitivePacketEncoder(0x01)
	p.SetFloat32Value(float32(model.Noise))
	buf := p.Encode()
	return 0x34, buf
}

func main() {
	sfn := yomo.NewStreamFunction(yomo.WithName("Noise"), yomo.WithZipperAddr("localhost:9000"))
	defer sfn.Close()

	// 开始监听 dataID 为 0x30~0x33 的数据
	sfn.SetObserveDataID(0x33)

	// 设置要执行的函数
	sfn.SetHandler(handler)

	// 开始执行
	err := sfn.Connect()
	if err != nil {
		log.Printf("[stream-fn] connect err=%v", err)
		os.Exit(1)
	}
	select {}
}
