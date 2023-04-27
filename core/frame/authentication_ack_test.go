package frame

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAuthenticationAckFrame(t *testing.T) {
	f := NewAuthenticationAckFrame()

	bytes := f.Encode()
	assert.Equal(t, []byte{0x91, 0x0}, bytes)

	got, err := DecodeToAuthenticationAckFrame(bytes)
	assert.Equal(t, f, got)
	assert.NoError(t, err)
	assert.EqualValues(t, TagOfAuthenticationAckFrame, f.Type())
}
