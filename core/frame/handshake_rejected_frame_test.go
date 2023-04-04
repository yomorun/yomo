package frame

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHandshakeRejectedFrame(t *testing.T) {
	var (
		id     = "asdfghjkl"
		reason = "123456"
	)

	f := NewHandshakeRejectedFrame(id, reason)

	buf := f.Encode()
	got, err := DecodeToHandshakeRejectedFrame(buf)

	assert.NoError(t, err)

	assert.Equal(t, id, got.StreamID())
	assert.Equal(t, reason, got.Reason())
}
