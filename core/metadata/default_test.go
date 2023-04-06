package metadata

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMetadata(t *testing.T) {
	encoder := DefaultDecoder()

	md, err := encoder.Decode([]byte{})
	assert.NoError(t, err)

	got, err := md.Encode()
	assert.NoError(t, err)
	assert.Equal(t, []byte{}, got)

	md = md.Merge(&Default{}, &Default{})

	got, err = md.Encode()
	assert.NoError(t, err)
	assert.Equal(t, []byte{}, got)
}
