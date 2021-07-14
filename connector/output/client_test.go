package output

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	y3 "github.com/yomorun/y3-codec-golang"
	"github.com/yomorun/yomo/core/rx"
	mockserver "github.com/yomorun/yomo/server/mock"
	mocksource "github.com/yomorun/yomo/source/mock"
)

func mockOutputConnector(t *testing.T, handler func(rxstream rx.Stream) rx.Stream) {
	// new a Output-Connector client.
	cli := New("test output connector")
	defer cli.Close()

	// connect to YoMo-Server.
	cli, err := cli.Connect(mockserver.IP, mockserver.Port)
	if err != nil {
		t.Errorf("[output] connect expected err is nil, but got %v", err)
	}

	// run handler
	go cli.Run(handler)
}

func TestReceiveRawData(t *testing.T) {
	// new a YoMo-Server.
	go mockserver.New()

	// test data
	data := []byte("test")

	// check if the raw data matches the test data sent from source.
	check := func(_ context.Context, i interface{}) (interface{}, error) {
		if !bytes.Equal(i.([]byte), data) {
			t.Errorf("[output] check data expected %v, but got %v", data, i)
		}
		return i, nil
	}

	// handler handles data in Rx way.
	handler := func(rxstream rx.Stream) rx.Stream {
		return rxstream.RawBytes().Map(check)
	}

	// run output-connector to receive data.
	mockOutputConnector(t, handler)

	// send the raw bytes from YoMo-Source to YoMo-Server.
	err := mocksource.SendDataToYoMoServer(data)
	if err != nil {
		t.Errorf("[output] SendDataToYoMoServer expected err is nil, but got %v", err)
	}
}

func TestReceiveDataWithY3Codec(t *testing.T) {
	// new a YoMo-Server.
	go mockserver.New()

	// test data with Y3 Codec
	var dataKey byte = 0x10
	codec := y3.NewCodec(dataKey)
	data := "test"
	buf, err := codec.Marshal(data)
	if err != nil {
		t.Errorf("[output] Y3 Marshal expected err is nil, but got %v", err)
	}

	// decode the data by Y3 Codec
	decode := func(v []byte) (interface{}, error) {
		return y3.ToUTF8String(v)
	}

	// check if the data matches the test data sent from source.
	check := func(_ context.Context, i interface{}) (interface{}, error) {
		if i.(string) != data {
			t.Errorf("[output] check data expected %v, but got %v", data, i)
		}
		return i, nil
	}

	// handler handles data in Rx way.
	handler := func(rxstream rx.Stream) rx.Stream {
		return rxstream.
			Subscribe(dataKey).
			OnObserve(decode).
			Map(check)
	}

	// run output-connector
	mockOutputConnector(t, handler)

	// send the Y3 encoded data from YoMo-Source to YoMo-Server.
	err = mocksource.SendDataToYoMoServer(buf)
	if err != nil {
		t.Errorf("[output] SendDataToYoMoServer expected err is nil, but got %v", err)
	}
}

func TestReceiveDataWithJSON(t *testing.T) {
	// new a YoMo-Server.
	go mockserver.New()

	// test data with JSON Codec
	type testData struct {
		Name string `json:"name"`
	}

	data := testData{
		Name: "test",
	}
	buf, err := json.Marshal(data)
	if err != nil {
		t.Errorf("[output] json Marshal expected err is nil, but got %v", err)
	}

	// check if the data matches the test data sent from source.
	check := func(_ context.Context, i interface{}) (interface{}, error) {
		if i.(testData).Name != data.Name {
			t.Errorf("[output] check data expected %v, but got %v", data, i)
		}
		return i, nil
	}

	// handler handles data in Rx way.
	handler := func(rxstream rx.Stream) rx.Stream {
		return rxstream.
			Unmarshal(json.Unmarshal, func() interface{} { return &testData{} }).
			Map(check)
	}

	// run output-connector
	mockOutputConnector(t, handler)

	// send the JSON encoded data from YoMo-Source to YoMo-Server.
	err = mocksource.SendDataToYoMoServer(buf)
	if err != nil {
		t.Errorf("[output] SendDataToYoMoServer expected err is nil, but got %v", err)
	}
}
