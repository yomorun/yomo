package frame

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHandshakeFrame(t *testing.T) {
	var (
		name            = "yomo"
		id              = "asdfghjkl"
		streamType      = byte(0x5F)
		observeDataTags = []Tag{'a', 'b', 'c'}
		metadata        = []byte{'d', 'e', 'f'}
	)

	f := NewHandshakeFrame(name, id, streamType, observeDataTags, metadata)

	buf := f.Encode()
	got, err := DecodeToHandshakeFrame(buf)

	assert.NoError(t, err)

	assert.Equal(t, name, got.Name())
	assert.Equal(t, id, got.ID())
	assert.Equal(t, streamType, got.StreamType())
	assert.Equal(t, observeDataTags, got.ObserveDataTags())
	assert.Equal(t, metadata, got.Metadata())
}
