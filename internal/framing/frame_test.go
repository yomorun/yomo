package framing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadFrameLength(t *testing.T) {
	t.Run("read length from frame bytes", func(t *testing.T) {
		input := []byte{0, 0, 5, 0, 129, 128, 83}
		// read length from first 3 bytes
		len, buf := ReadFrameLength(input)
		assert.Equal(t, 5, len)
		assert.Equal(t, input, buf)
	})

	t.Run("skip 0 from frame bytes", func(t *testing.T) {
		input := []byte{0, 0, 0, 0, 5, 129, 128, 83}
		expected := []byte{0, 0, 5, 129, 128, 83}
		len, buf := ReadFrameLength(input)
		assert.Equal(t, 5, len)
		assert.Equal(t, expected, buf)
	})
}

func TestGetRawBytesWithoutFraming(t *testing.T) {
	t.Run("return original bytes when the length <= 3", func(t *testing.T) {
		input := []byte{1, 2, 3}
		actual := GetRawBytesWithoutFraming(input)
		assert.Equal(t, input, actual)
	})
	t.Run("should remove the framing bytes", func(t *testing.T) {
		input := []byte{0, 0, 3, 0, 1, 2, 3}
		actual := GetRawBytesWithoutFraming(input)
		expected := []byte{1, 2, 3}
		assert.Equal(t, expected, actual)
	})
}

func TestFromBytes(t *testing.T) {
	t.Run("return error when empty bytes", func(t *testing.T) {
		buf := []byte{}
		header, err := FromRawBytes(buf)

		assert.Nil(t, header)
		assert.Error(t, err)
	})

	t.Run("get frame from bytes", func(t *testing.T) {
		buf := []byte{6, 1, 2, 3}
		f, err := FromRawBytes(buf)

		assert.Equal(t, FrameTypePayload, f.Type())
		assert.Equal(t, []byte{1, 2, 3}, f.Data())
		assert.Nil(t, err)
	})
}
