package framing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBytesFromPayloadFrame(t *testing.T) {
	t.Run("Payload Frame without metadata", func(t *testing.T) {
		data := []byte{50, 1, 0}
		frame := NewPayloadFrame(data)
		actual := frame.Bytes()
		expected := []byte{0, 0, 6, byte(FrameTypePayload), 0, 0, 50, 1, 0}

		assert.Equal(t, expected, actual)
		assert.Equal(t, FrameTypePayload, frame.Type())
	})

	t.Run("Payload Frame with metadata", func(t *testing.T) {
		data := []byte{50, 1, 0}
		metadata := []byte{1, 2, 3}
		frame := NewPayloadFrame(data, WithMetadata(metadata))
		actual := frame.Bytes()
		expected := []byte{0, 0, 9, byte(FrameTypePayload), 0, 3, 1, 2, 3, 50, 1, 0}

		assert.Equal(t, expected, actual)
		assert.Equal(t, FrameTypePayload, frame.Type())
		assert.Equal(t, metadata, frame.Metadata())
		assert.Equal(t, data, frame.data)
	})
}
