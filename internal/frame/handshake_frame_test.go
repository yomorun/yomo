package frame

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHandshakeFrameEncode(t *testing.T) {
	expectedName := "1234"
	expectedType := "yomo"
	m := NewHandshakeFrame(expectedName, expectedType)
	assert.Equal(t, []byte{
		0x80 | byte(TagOfHandshakeFrame), 0x0C,
		byte(TagOfHandshakeName), 0x04, 0x31, 0x32, 0x33, 0x34,
		byte(TagOfHandshakeType), 0x04, 0x79, 0x6F, 0x6D, 0x6F}, m.Encode())

	Handshake, err := DecodeToHandshakeFrame(m.Encode())
	assert.NoError(t, err)
	assert.EqualValues(t, expectedName, Handshake.Name)
	assert.EqualValues(t, expectedType, Handshake.ClientType)
}
