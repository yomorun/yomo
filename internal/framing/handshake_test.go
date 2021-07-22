package framing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetBytesAndTypeFromHandshakeFrame(t *testing.T) {
	data := []byte{1, 2, 3}
	frame := NewHandshakeFrame(data)
	actual := frame.Bytes()
	expected := []byte{0, 0, 4, byte(FrameTypeHandshake), 1, 2, 3}

	assert.Equal(t, expected, actual)
	assert.Equal(t, FrameTypeHandshake, frame.Type())
}
