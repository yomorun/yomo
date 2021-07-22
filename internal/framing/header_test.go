package framing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHeaderFromBytes(t *testing.T) {
	t.Run("return error when empty bytes", func(t *testing.T) {
		buf := []byte{}
		header, err := HeaderFromBytes(buf)

		assert.Nil(t, header)
		assert.Error(t, err)
	})

	t.Run("get header from bytes", func(t *testing.T) {
		buf := []byte{1, 1, 2, 3, 4}
		header, err := HeaderFromBytes(buf)

		assert.Equal(t, FrameTypeHeartbeat, header.FrameType)
		assert.Nil(t, err)
	})
}
