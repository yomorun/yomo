package frame

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRejectedFrameEncode(t *testing.T) {
	f := NewRejectedFrame()
	assert.Equal(t, []byte{0x80 | byte(TagOfRejectedFrame), 0x00}, f.Encode())
}

func TestRejectedFrameDecode(t *testing.T) {
	buf := []byte{0x80 | byte(TagOfRejectedFrame), 0x00}
	ping, err := DecodeToRejectedFrame(buf)
	assert.NoError(t, err)
	assert.Equal(t, []byte{0x80 | byte(TagOfRejectedFrame), 0x00}, ping.Encode())
}
