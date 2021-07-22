package framing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBytesFromPayloadFrame(t *testing.T) {
	data := []byte{50, 1, 0}
	frame := NewPayloadFrame(data)
	actual := frame.Bytes()
	expected := []byte{0, 0, 4, byte(FrameTypePayload), 50, 1, 0}

	assert.Equal(t, expected, actual)
	assert.Equal(t, FrameTypePayload, frame.Type())
}
