package framing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetTypeFromHeartbeatFrame(t *testing.T) {
	frame := NewHeartbeatFrame()
	expected := FrameTypeHeartbeat
	actual := frame.Type()

	assert.Equal(t, expected, actual)
}
