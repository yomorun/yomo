package frame

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var handShakeAckTestBuf = []byte{0x80 | byte(TagOfHandshakeAckFrame), 0}

func TestHandshakeAckFrameEncode(t *testing.T) {
	f := NewHandshakeAckFrame()
	assert.Equal(t, handShakeAckTestBuf, f.Encode())
}

func TestHandshakeAckFrameDecode(t *testing.T) {
	f, err := DecodeToHandshakeAckFrame(handShakeAckTestBuf)
	assert.NoError(t, err)
	assert.Equal(t, handShakeAckTestBuf, f.Encode())
}
