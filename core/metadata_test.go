package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMetadata(t *testing.T) {
	md := NewDefaultMetadata("source", true, "xxxxxxx", "sssssss")

	assert.Equal(t, "source", GetSourceIDFromMetadata(md))
	assert.Equal(t, true, GetBroadcastFromMetadata(md))
	assert.Equal(t, "xxxxxxx", GetTIDFromMetadata(md))
	assert.Equal(t, "sssssss", GetSIDFromMetadata(md))
}
