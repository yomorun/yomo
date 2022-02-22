package frame

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHandshakeFrameEncode(t *testing.T) {
	expectedName := "1234"
	var expectedType byte = 0xD3
	m := NewHandshakeFrame(expectedName, expectedType, []byte{0x01, 0x02}, "", 0x0, nil)
	assert.Equal(t, []byte{
		0x80 | byte(TagOfHandshakeFrame), 0x14,
		byte(TagOfHandshakeName), 0x04, 0x31, 0x32, 0x33, 0x34,
		byte(TagOfHandshakeType), 0x01, 0xD3,
		byte(TagOfHandshakeObserved), 0x02, 0x01, 0x02,
		byte(TagOfHandshakeAppID), 0x0,
		byte(TagOfHandshakeAuthType), 0x01, 0x0,
		byte(TagOfHandshakeAuthPayload), 0x0,
	},
		m.Encode(),
	)

	Handshake, err := DecodeToHandshakeFrame(m.Encode())
	assert.NoError(t, err)
	assert.EqualValues(t, expectedName, Handshake.Name)
	assert.EqualValues(t, expectedType, Handshake.ClientType)
}
