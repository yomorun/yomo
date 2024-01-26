package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core/metadata"
)

func TestMetadata(t *testing.T) {
	md := NewMetadata("source", "tid", "traceID", "spanID", true)

	SetMetadataTarget(md, "target")
	v, ok := md.Get(metadata.TargetKey)
	assert.True(t, ok)
	assert.Equal(t, "target", v)

	assert.Equal(t, "tid", GetTIDFromMetadata(md))
	assert.Equal(t, true, GetTracedFromMetadata(md))
}
