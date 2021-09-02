package frame

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMetaFrameEncode(t *testing.T) {
	m := NewMetaFrame("1234", "issuer")
	assert.Equal(t, []byte{0x80 | byte(TagOfMetaFrame), 0x06, byte(TagOfTransactionID), 0x04, 0x31, 0x32, 0x33, 0x34}, m.Encode())
}

func TestMetaFrameDecode(t *testing.T) {
	buf := []byte{0x80 | byte(TagOfMetaFrame), 0x06, byte(TagOfTransactionID), 0x04, 0x31, 0x32, 0x33, 0x34}
	meta, err := DecodeToMetaFrame(buf)
	assert.NoError(t, err)
	assert.EqualValues(t, "1234", meta.TransactionID())
}
