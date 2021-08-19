package streamfunction

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/y3-codec-golang"
	"github.com/yomorun/yomo/core/rx"
	mocksource "github.com/yomorun/yomo/source/mock"
	mockserver "github.com/yomorun/yomo/zipper/mock"
)

const testFuncName = "test stream function"

func mockStreamFn(t *testing.T, handler func(rxstream rx.Stream) rx.Stream) {
	// new a Stream-Function client.
	cli := New(testFuncName)
	defer cli.Close()

	// connect to YoMo-Zipper.
	cli, err := cli.Connect(mockserver.IP, mockserver.Port)
	if err != nil {
		t.Errorf("[stream-fn] connect expected err is nil, but got %v", err)
	}

	// pipe handler into rx.Stream.
	go cli.Pipe(handler)
}

func TestProcessRawData(t *testing.T) {
	// new a YoMo-Zipper.
	go mockserver.NewWithFuncName(testFuncName)

	// test data
	data := []byte("test")

	// check if the raw data matches the test data sent from source.
	check := func(_ context.Context, i interface{}) (interface{}, error) {
		b := i.([]byte)
		assert.Equal(t, data, b)

		// convert bytes to string.
		return string(b), nil
	}

	// handler handles data in Rx way.
	handler := func(rxstream rx.Stream) rx.Stream {
		return rxstream.
			RawBytes().
			Map(check).
			StdOut()
	}

	// run Stream-Function to process the data in real-time.
	mockStreamFn(t, handler)

	// send the raw bytes from YoMo-Source to YoMo-Zipper.
	err := mocksource.SendDataToYoMoServer(data)
	if err != nil {
		t.Errorf("[stream-fn] SendDataToYoMoServer expected err is nil, but got %v", err)
	}
}

func TestProcessDataWithY3Codec(t *testing.T) {
	// new a YoMo-Zipper.
	go mockserver.New()

	// test data
	var dataKey byte = 0x10
	codec := y3.NewCodec(dataKey)
	data := "test"
	// encode data with Y3 Codec
	buf, err := codec.Marshal(data)
	if err != nil {
		t.Errorf("[stream-fn] Y3 Marshal expected err is nil, but got %v", err)
	}

	// decode the data by Y3 Codec
	decode := func(v []byte) (interface{}, error) {
		return y3.ToUTF8String(v)
	}

	// check if the data matches the test data sent from source.
	check := func(_ context.Context, i interface{}) (interface{}, error) {
		assert.Equal(t, data, i)
		return i, nil
	}

	// handler handles data in Rx way.
	handler := func(rxstream rx.Stream) rx.Stream {
		return rxstream.
			Subscribe(dataKey).
			OnObserve(decode).
			Map(check).
			Encode(0x11) // append a new data with Y3 Codec.
	}

	// run Stream-Function to process the data in real-time.
	mockStreamFn(t, handler)

	// send the Y3 encoded data from YoMo-Source to YoMo-Zipper.
	err = mocksource.SendDataToYoMoServer(buf)
	if err != nil {
		t.Errorf("[stream-fn] SendDataToYoMoServer expected err is nil, but got %v", err)
	}
}

func TestReceiveDataWithJSON(t *testing.T) {
	// new a YoMo-Zipper.
	go mockserver.New()

	// test data
	type testData struct {
		Name string `json:"name"`
	}

	data := testData{
		Name: "test",
	}
	// encode the code with JSON Codec
	buf, err := json.Marshal(data)
	if err != nil {
		t.Errorf("[stream-fn] json Marshal expected err is nil, but got %v", err)
	}

	// check if the data matches the test data sent from source.
	check := func(_ context.Context, i interface{}) (interface{}, error) {
		actual := i.(*testData)
		assert.Equal(t, data, *actual)
		return i, nil
	}

	// handler handles data in Rx way.
	handler := func(rxstream rx.Stream) rx.Stream {
		return rxstream.
			Unmarshal(json.Unmarshal, func() interface{} { return &testData{} }).
			Map(check).
			Marshal(json.Marshal) // append a new data with JSON Codec.
	}

	// run Stream-Function to process the data in real-time.
	mockStreamFn(t, handler)

	// send the JSON encoded data from YoMo-Source to YoMo-Zipper.
	err = mocksource.SendDataToYoMoServer(buf)
	if err != nil {
		t.Errorf("[stream-fn] SendDataToYoMoServer expected err is nil, but got %v", err)
	}
}
