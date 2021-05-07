package main

import (
	"context"
	"fmt"

	"github.com/yomorun/yomo/pkg/client"

	y3 "github.com/yomorun/y3-codec-golang"
	"github.com/yomorun/yomo/pkg/rx"
)

const DataAKey = 0x3a
const DataBKey = 0x3b

var callbacka = func(v []byte) (interface{}, error) {
	return y3.ToFloat32(v)
}

var callbackb = func(v []byte) (interface{}, error) {
	return y3.ToUTF8String(v)
}

var printera = func(_ context.Context, i interface{}) (interface{}, error) {
	value := i.(float32)
	fmt.Println(fmt.Sprintf("[%s]> value: %f", "data-a", value))
	return i, nil
}

var printerb = func(_ context.Context, i interface{}) (interface{}, error) {
	value := i.(string)
	fmt.Println(fmt.Sprintf("[%s]> value: %s", "data-b", value))
	return i, nil
}

var zipper = func(_ context.Context, ia interface{}, ib interface{}) (interface{}, error) {

	return fmt.Sprintf("⚡️ Zip [%s],[%s] -> Value: %f, %s", "dataA", "dataB", ia.(float32), ib.(string)), nil
}

// Handler handle two event streams and calculate sum when data arrived
func Handler(rxstream rx.RxStream) rx.RxStream {
	streamA := rxstream.Subscribe(DataAKey).OnObserve(callbacka).Map(printera)
	streamB := rxstream.Subscribe(DataBKey).OnObserve(callbackb).Map(printerb)

	stream := streamA.ZipFromIterable(streamB, zipper).StdOut().Encode(0x10)
	return stream
}

func main() {
	st, err := client.NewServerless("training", "localhost", 9000).Connect()
	defer st.Close()

	if err != nil {
		panic(err)
	}

	st.Pipe(Handler)
}
