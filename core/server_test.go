package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
	_ "github.com/yomorun/yomo/pkg/auth"
)

func TestMakeSourceTagFindConnectionFunc(t *testing.T) {
	findFunc := sourceIDTagFindConnectionFunc("hello", frame.Tag(7))

	t.Run("find successful", func(t *testing.T) {
		source := &mockConnectionInfo{id: "hello", observed: []frame.Tag{frame.Tag(7)}, streamType: StreamTypeSource}
		got := findFunc(source)
		assert.True(t, got)
	})

	t.Run("find in name failed", func(t *testing.T) {
		source := &mockConnectionInfo{id: "olleh", observed: []frame.Tag{frame.Tag(7)}, streamType: StreamTypeSource}
		got := findFunc(source)
		assert.False(t, got)
	})

	t.Run("find in tag failed", func(t *testing.T) {
		source := &mockConnectionInfo{id: "hello", observed: []frame.Tag{frame.Tag(6)}, streamType: StreamTypeSource}
		got := findFunc(source)
		assert.False(t, got)
	})
}

type mockConnectionInfo struct {
	name       string
	id         string
	streamType StreamType
	metadata   metadata.M
	observed   []frame.Tag
}

func (s *mockConnectionInfo) ID() string                   { return s.id }
func (s *mockConnectionInfo) Name() string                 { return s.name }
func (s *mockConnectionInfo) Metadata() metadata.M         { return s.metadata }
func (s *mockConnectionInfo) StreamType() StreamType       { return s.streamType }
func (s *mockConnectionInfo) ObserveDataTags() []frame.Tag { return s.observed }
