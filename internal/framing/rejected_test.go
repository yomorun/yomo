package framing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetTypeFromRejectedFrame(t *testing.T) {
	frame := NewRejectedFrame()
	expected := FrameTypeRejected
	actual := frame.Type()

	assert.Equal(t, expected, actual)
}
