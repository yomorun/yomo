package decoder

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

// observeFunc is the callback function for `OnObserve` in Y3 Decoder.
var observeFunc = func(v []byte) (interface{}, error) {
	return v, nil
}

func TestReadRawBytesFromDecoder(t *testing.T) {
	data := "test"

	// mock observable
	observable := FromItems([]interface{}{data})
	// get raw bytes
	bytesCh := observable.RawBytes()
	expected := []byte(data)
	for b := range bytesCh {
		assert.Equal(t, expected, b)
		break
	}
}

func TestObserveDataFromY3Decoder(t *testing.T) {
	t.Run("Observe data from Y3 Node Packet", func(t *testing.T) {
		data := []byte{129, 3, 144, 25, 17, 4, 65, 233, 15, 156, 18, 6, 175, 170, 192, 218, 156, 100, 19, 9, 108, 111, 99, 97, 108, 104, 111, 115, 116}

		// mock observable
		observable := FromItems([]interface{}{data})
		var key byte = 0x10
		ch := observable.
			Subscribe(key).
			OnObserve(observeFunc)

		expected := []byte{144, 25, 17, 4, 65, 233, 15, 156, 18, 6, 175, 170, 192, 218, 156, 100, 19, 9, 108, 111, 99, 97, 108, 104, 111, 115, 116}
		for actual := range ch {
			assert.Equal(t, expected, actual)
			break
		}
	})

	t.Run("Observe data from Y3 Primitive Packet", func(t *testing.T) {
		data := []byte{5, 16, 3, 1, 2, 3}

		// mock observable
		observable := FromItems([]interface{}{data})
		var key byte = 0x10
		ch := observable.
			Subscribe(key).
			OnObserve(observeFunc)

		expected := []byte{1, 2, 3}
		for actual := range ch {
			assert.Equal(t, expected, actual)
			break
		}
	})

	t.Run("Observe data by multi keys", func(t *testing.T) {
		data := []byte{16, 3, 1, 2, 3, 17, 2, 1, 2}

		// mock observable
		observable := FromItems([]interface{}{data})

		// multi keys
		var key1 byte = 0x10
		var key2 byte = 0x11

		// multi subscribe
		checkMap := map[byte]OnObserveFunc{
			key1: observeFunc,
			key2: observeFunc,
		}
		kvCh := observable.MultiSubscribe(key1, key2).OnMultiObserve(checkMap)

		count := 1
		for actual := range kvCh {
			// kv 1
			if count == 1 {
				assert.Equal(t, key1, actual.Key)
				assert.Equal(t, []byte{1, 2, 3}, actual.Value)
			}
			// kv 2
			if count == 2 {
				assert.Equal(t, key2, actual.Key)
				assert.Equal(t, []byte{1, 2}, actual.Value)
			}
			count++
		}
	})
}

func TestNotObservefFromY3Decoder(t *testing.T) {
	t.Run("key is not matched from Y3 Node Packet", func(t *testing.T) {
		data := []byte{129, 3, 144, 25, 17, 4, 65, 233, 15, 156, 18, 6, 175, 170, 192, 218, 156, 100, 19, 9, 108, 111, 99, 97, 108, 104, 111, 115, 116}

		// mock observable
		observable := FromItems([]interface{}{data})
		var key byte = 0x20
		ch := observable.
			Subscribe(key).
			OnObserve(observeFunc)

		hasValue := false
		for actual := range ch {
			if actual != nil {
				hasValue = true
			}
		}

		assert.False(t, hasValue)
	})
	t.Run("key is not matched from Y3 Primitive Packet", func(t *testing.T) {
		data := []byte{16, 3, 1, 2, 3}

		// mock observable
		observable := FromItems([]interface{}{data})
		var key byte = 0x20
		ch := observable.
			Subscribe(key).
			OnObserve(observeFunc)

		hasValue := false
		for actual := range ch {
			if actual != nil {
				hasValue = true
			}
		}

		assert.False(t, hasValue)
	})
}

func TestUnmarshalFromJSONDecoder(t *testing.T) {
	type testData struct {
		Name string `json:"name"`
	}

	data := testData{
		Name: "test",
	}

	// JSON Marshal
	dataBuf, err := json.Marshal(data)
	assert.NoError(t, err)

	// mock observable
	observable := FromItems([]interface{}{dataBuf})

	ch := observable.
		Unmarshal(json.Unmarshal, func() interface{} { return &testData{} })

	for actual := range ch {
		assert.Equal(t, &data, actual)
		break
	}
}
