package metadata

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMetadata(t *testing.T) {
	builder := DefaultBuilder()

	m, err := builder.Build(nil)

	assert.NoError(t, err)
	assert.Equal(t, []uint8([]byte(nil)), m.Encode())

	de, err := builder.Decode([]byte{})

	assert.NoError(t, err)
	assert.Equal(t, m, de)
}
