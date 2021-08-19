package framing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadFrameLength(t *testing.T) {
	t.Run("read length from frame bytes", func(t *testing.T) {
		input := []byte{0, 0, 7, 0, 0, 0, 129, 128, 83}
		// read length from first 3 bytes
		len := ReadFrameLength(input)
		assert.Equal(t, 7, len)
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
		buf := []byte{6, 0, 0, 1, 2, 3}
		f, err := FromRawBytes(buf)

		assert.Equal(t, FrameTypePayload, f.Type())
		assert.Equal(t, []byte{1, 2, 3}, f.Data())
		assert.Nil(t, err)
	})
}
