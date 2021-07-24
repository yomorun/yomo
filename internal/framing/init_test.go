package framing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetTypeFromInitFrame(t *testing.T) {
	frame := NewInitFrame()
	expected := FrameTypeInit
	actual := frame.Type()

	assert.Equal(t, expected, actual)
}
