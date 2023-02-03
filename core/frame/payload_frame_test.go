package frame

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPayloadFrameEncode(t *testing.T) {
	f := &PayloadFrame{
		Tag(0x13),
		[]byte("yomo"),
	}
	f.SetCarriage([]byte("yomo"))
	assert.Equal(t, []byte{0x80 | byte(TagOfPayloadFrame), 0x9, 0x1, 0x1, 0x13, 0x2, 0x04, 0x79, 0x6F, 0x6D, 0x6F}, f.Encode())
}

func TestPayloadFrameDecode(t *testing.T) {
	buf := []byte{0x80 | byte(TagOfPayloadFrame), 0x9, 0x1, 0x1, 0x13, 0x2, 0x04, 0x79, 0x6F, 0x6D, 0x6F}
	payload := new(PayloadFrame)
	err := DecodeToPayloadFrame(buf, payload)
	assert.NoError(t, err)
	assert.EqualValues(t, 0x13, payload.Tag)
	assert.Equal(t, []byte{0x79, 0x6F, 0x6D, 0x6F}, payload.Carriage)
}
