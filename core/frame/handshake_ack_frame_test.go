package frame

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var handShakeAckTestBuf = []byte{0x80 | byte(TagOfHandshakeAckFrame), 0x8, 0x28, 0x6, 0x74, 0x68, 0x65, 0x2d, 0x69, 0x64}

var testStreamID = "the-id"

func TestHandshakeAckFrameEncode(t *testing.T) {
	f := NewHandshakeAckFrame(testStreamID)
	assert.Equal(t, TagOfHandshakeAckFrame, f.Type())
	assert.Equal(t, handShakeAckTestBuf, f.Encode())
}

func TestHandshakeAckFrameDecode(t *testing.T) {
	f, err := DecodeToHandshakeAckFrame(handShakeAckTestBuf)
	assert.NoError(t, err)
	assert.Equal(t, TagOfHandshakeAckFrame, f.Type())
	assert.Equal(t, testStreamID, f.StreamID())
	assert.Equal(t, handShakeAckTestBuf, f.Encode())
}
