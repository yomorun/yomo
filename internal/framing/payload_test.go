package framing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBytesFromPayloadFrame(t *testing.T) {
	data := []byte{50, 1, 0}
	frame := NewPayloadFrame(data)
	actual := frame.Bytes()
	expected := []byte{0, 0, 3, 50, 1, 0}

	assert.Equal(t, expected, actual)
}
