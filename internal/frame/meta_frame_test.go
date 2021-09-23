package frame

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMetaFrameEncode(t *testing.T) {
	m := NewMetaFrame()
	tidbuf := []byte(m.tid)
	result := []byte{0x80 | byte(TagOfMetaFrame), byte(1 + 1 + len(tidbuf)), byte(TagOfTransactionID), byte(len(tidbuf))}
	result = append(result, tidbuf...)
	assert.Equal(t, result, m.Encode())
}

func TestMetaFrameDecode(t *testing.T) {
	buf := []byte{0x80 | byte(TagOfMetaFrame), 0x06, byte(TagOfTransactionID), 0x04, 0x31, 0x32, 0x33, 0x34}
	meta, err := DecodeToMetaFrame(buf)
	assert.NoError(t, err)
	assert.EqualValues(t, "1234", meta.TransactionID())
}
