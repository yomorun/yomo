package frame

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHandshakeFrameEncode(t *testing.T) {
	expectedName := "1234"
	var expectedType byte = 0xD3
	m := NewHandshakeFrame(expectedName, expectedType, []byte{0x01, 0x02}, "token", nil)
	assert.Equal(t, []byte{
		0x80 | byte(TagOfHandshakeFrame), 0x16,
		byte(TagOfHandshakeName), 0x04, 0x31, 0x32, 0x33, 0x34,
		byte(TagOfHandshakeType), 0x01, 0xD3,
		byte(TagOfHandshakeObserveDataTags), 0x02, 0x01, 0x02,
		// byte(TagOfHandshakeAppID), 0x0,
		byte(TagOfHandshakeAuthName), 0x05, 0x74, 0x6f, 0x6b, 0x65, 0x6e,
		byte(TagOfHandshakeAuthPayload), 0x0,
	},
		m.Encode(),
	)

	Handshake, err := DecodeToHandshakeFrame(m.Encode())
	assert.NoError(t, err)
	assert.EqualValues(t, expectedName, Handshake.Name)
	assert.EqualValues(t, expectedType, Handshake.ClientType)
}
