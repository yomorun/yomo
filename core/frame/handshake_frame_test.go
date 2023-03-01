package frame

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHandshakeFrame(t *testing.T) {
	f := &HandshakeFrame{
		Name:            "yomo",
		ID:              "asdfghjkl",
		StreamType:      0x5F,
		ObserveDataTags: []Tag{'a', 'b', 'c'},
		Metadata:        []byte{'d', 'e', 'f'},
	}

	buf := f.Encode()
	got, err := DecodeToHandshakeFrame(buf)

	assert.NoError(t, err)
	assert.Equal(t, f, got)
}
