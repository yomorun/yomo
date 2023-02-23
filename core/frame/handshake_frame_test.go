package frame

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHandshakeFrameEncode(t *testing.T) {
	m := NewHandshakeFrame("token", "a")
	assert.Equal(t, []byte{
		0x80 | byte(TagOfHandshakeFrame), 0xa,
		byte(TagOfHandshakeAuthName), 0x05, 0x74, 0x6f, 0x6b, 0x65, 0x6e,
		byte(TagOfHandshakeAuthPayload), 0x01, 0x61,
	},
		m.Encode(),
	)

	Handshake, err := DecodeToHandshakeFrame(m.Encode())
	assert.NoError(t, err)
	assert.EqualValues(t, "token", Handshake.AuthName())
	assert.EqualValues(t, "a", Handshake.AuthPayload())
}
