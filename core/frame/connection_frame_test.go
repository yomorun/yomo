package frame

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConnectionFrame(t *testing.T) {
	f := &ConnectionFrame{
		Name:            "yomo",
		ClientID:        "asdfghjkl",
		ClientType:      0x5F,
		ObserveDataTags: []Tag{'a', 'b', 'c'},
		Metadata:        []byte{'d', 'e', 'f'},
	}

	buf := f.Encode()
	got, err := DecodeToConnectionFrame(buf)

	assert.NoError(t, err)
	assert.Equal(t, f, got)
}
