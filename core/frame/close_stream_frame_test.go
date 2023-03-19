package frame

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCloseStreamFrame(t *testing.T) {
	f := NewCloseStreamFrame("eeffgg", "aabbcc")

	bytes := f.Encode()
	assert.Equal(t, []byte{0x94, 0x10, 0x15, 0x6, 0x65, 0x65, 0x66, 0x66, 0x67, 0x67, 0x16, 0x6, 0x61, 0x61, 0x62, 0x62, 0x63, 0x63}, bytes)

	got, err := DecodeToCloseStreamFrame(bytes)
	assert.Equal(t, f, got)
	assert.NoError(t, err)
	assert.EqualValues(t, "eeffgg", f.StreamID())
	assert.EqualValues(t, "aabbcc", f.Reason())
}
