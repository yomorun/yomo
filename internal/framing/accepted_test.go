package framing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetTypeFromAcceptedFrame(t *testing.T) {
	frame := NewAcceptedFrame()
	expected := FrameTypeAccepted
	actual := frame.Type()

	assert.Equal(t, expected, actual)
}
