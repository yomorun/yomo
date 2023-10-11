package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMetadata(t *testing.T) {
	md := NewDefaultMetadata("source", "xxxxxxx", "sssssss", true)

	assert.Equal(t, "source", GetSourceIDFromMetadata(md))
	assert.Equal(t, "xxxxxxx", GetTIDFromMetadata(md))
	assert.Equal(t, "sssssss", GetSIDFromMetadata(md))
	assert.Equal(t, true, GetTracedFromMetadata(md))

	SetTIDToMetadata(md, "ccccccc")
	assert.Equal(t, "ccccccc", GetTIDFromMetadata(md))

	SetSIDToMetadata(md, "aaaaaaa")
	assert.Equal(t, "aaaaaaa", GetSIDFromMetadata(md))

	SetTracedToMetadata(md, false)
	assert.Equal(t, false, GetTracedFromMetadata(md))
}
