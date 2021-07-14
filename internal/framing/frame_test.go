package framing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetFrameLengthBytes(t *testing.T) {
	t.Run("get bytes from Frame Length", func(t *testing.T) {
		actual := getFrameLengthBytes(3)
		expected := []byte{0, 0, 3}
		assert.Equal(t, expected, actual)
	})

	t.Run("get empty bytes when len <=0", func(t *testing.T) {
		actual := getFrameLengthBytes(0)
		expected := []byte{0, 0, 0}
		assert.Equal(t, expected, actual)

		// len < 0
		actual = getFrameLengthBytes(-10)
		assert.Equal(t, expected, actual)
	})
}

func TestReadFrameLength(t *testing.T) {
	t.Run("read length from frame bytes", func(t *testing.T) {
		input := []byte{0, 0, 5, 129, 128, 83}
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
