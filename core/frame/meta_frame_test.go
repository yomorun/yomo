package frame

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMetaFrameEncode(t *testing.T) {
	m := NewMetaFrame()
	tidbuf := []byte(m.tid)
	result := []byte{0x80 | byte(TagOfMetaFrame), byte(1 + 1 + len(tidbuf) + 2), byte(TagOfTransactionID), byte(len(tidbuf))}
	result = append(result, tidbuf...)
	result = append(result, byte(TagOfSourceID), 0x0)
	assert.Equal(t, result, m.Encode())
}

func TestMetaFrameDecode(t *testing.T) {
	buf := []byte{0x80 | byte(TagOfMetaFrame), 0x09, byte(TagOfTransactionID), 0x04, 0x31, 0x32, 0x33, 0x34, byte(TagOfSourceID), 0x01, 0x31}
	meta, err := DecodeToMetaFrame(buf)
	assert.NoError(t, err)
	assert.EqualValues(t, "1234", meta.TransactionID())
	assert.EqualValues(t, "1", meta.SourceID())
	t.Logf("%# x", buf)
}
