package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMetadata(t *testing.T) {
	md := NewMetadata("source", "tid", "traceID", "spanID", true)

	assert.Equal(t, "source", GetSourceIDFromMetadata(md))
	assert.Equal(t, "tid", GetTIDFromMetadata(md))
	assert.Equal(t, true, GetTracedFromMetadata(md))
}
