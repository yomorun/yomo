package framing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetTypeFromAckFrame(t *testing.T) {
	frame := NewAckFrame()
	expected := FrameTypeAck
	actual := frame.Type()

	assert.Equal(t, expected, actual)
}
