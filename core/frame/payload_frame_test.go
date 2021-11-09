package frame

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPayloadFrameEncode(t *testing.T) {
	f := NewPayloadFrame(0x13).SetCarriage([]byte("yomo"))
	assert.Equal(t, []byte{0x80 | byte(TagOfPayloadFrame), 0x06, 0x13, 0x04, 0x79, 0x6F, 0x6D, 0x6F}, f.Encode())
}

func TestPayloadFrameDecode(t *testing.T) {
	buf := []byte{0x80 | byte(TagOfPayloadFrame), 0x06, 0x13, 0x04, 0x79, 0x6F, 0x6D, 0x6F}
	payload, err := DecodeToPayloadFrame(buf)
	assert.NoError(t, err)
	assert.EqualValues(t, 0x13, payload.Sid)
	assert.Equal(t, []byte{0x79, 0x6F, 0x6D, 0x6F}, payload.Carriage)
}
