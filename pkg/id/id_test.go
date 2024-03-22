package id

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	t.Run("new a random id", func(t *testing.T) {
		str := New()
		assert.IsType(t, "", str)
		assert.Equal(t, 21, len(str))
	})
}
