package frame

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAuthenticationAckFrame(t *testing.T) {
	f := NewAuthenticationAckFrame(false, "aabbcc")

	bytes := f.Encode()
	assert.Equal(t, []byte{0x81, 0xb, 0x2, 0x1, 0x0, 0x3, 0x6, 0x61, 0x61, 0x62, 0x62, 0x63, 0x63}, bytes)

	got, err := DecodeToAuthenticationAckFrame(bytes)
	assert.Equal(t, f, got)
	assert.NoError(t, err)
	assert.EqualValues(t, false, f.OK())
	assert.EqualValues(t, "aabbcc", f.Reason())
}
