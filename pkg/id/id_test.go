package id

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	str := New()
	assert.IsType(t, "", str)
}
