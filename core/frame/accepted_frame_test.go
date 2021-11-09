package frame

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAcceptedFrameEncode(t *testing.T) {
	f := NewAcceptedFrame()
	assert.Equal(t, []byte{0x80 | byte(TagOfAcceptedFrame), 0x00}, f.Encode())
}

func TestAcceptedFrameDecode(t *testing.T) {
	buf := []byte{0x80 | byte(TagOfAcceptedFrame), 0x00}
	ping, err := DecodeToAcceptedFrame(buf)
	assert.NoError(t, err)
	assert.Equal(t, []byte{0x80 | byte(TagOfAcceptedFrame), 0x00}, ping.Encode())
}
