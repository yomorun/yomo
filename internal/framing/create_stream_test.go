package framing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetTypeFromCreateStreamFrame(t *testing.T) {
	frame := NewCreateStreamFrame()
	expected := FrameTypeCreateStream
	actual := frame.Type()

	assert.Equal(t, expected, actual)
}
