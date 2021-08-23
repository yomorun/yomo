package frame

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPingFrameEncode(t *testing.T) {
	f := NewPingFrame()
	assert.Equal(t, []byte{0x80 | byte(TagOfPingFrame), 0x00}, f.Encode())
}

func TestPingFrameDecode(t *testing.T) {
	buf := []byte{0x80 | byte(TagOfPingFrame), 0x00}
	ping, err := DecodeToPingFrame(buf)
	assert.NoError(t, err)
	assert.Equal(t, []byte{0x80 | byte(TagOfPingFrame), 0x00}, ping.Encode())
}

func TestPongFrameEncode(t *testing.T) {
	f := NewPongFrame()
	assert.Equal(t, []byte{0x80 | byte(TagOfPongFrame), 0x00}, f.Encode())
}

func TestPongFrameDecode(t *testing.T) {
	buf := []byte{0x80 | byte(TagOfPingFrame), 0x00}
	ping, err := DecodeToPongFrame(buf)
	assert.NoError(t, err)
	assert.Equal(t, []byte{0x80 | byte(TagOfPongFrame), 0x00}, ping.Encode())
}
