package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
	_ "github.com/yomorun/yomo/pkg/auth"
)

func TestMakeSourceTagFindStreamFunc(t *testing.T) {
	findFunc := sourceIDTagFindStreamFunc("hello", frame.Tag(7))

	t.Run("find successful", func(t *testing.T) {
		source := &mockStreamInfo{id: "hello", observed: []frame.Tag{frame.Tag(7)}, clientType: ClientTypeSource}
		got := findFunc(source)
		assert.True(t, got)
	})

	t.Run("find in name failed", func(t *testing.T) {
		source := &mockStreamInfo{id: "olleh", observed: []frame.Tag{frame.Tag(7)}, clientType: ClientTypeSource}
		got := findFunc(source)
		assert.False(t, got)
	})

	t.Run("find in tag failed", func(t *testing.T) {
		source := &mockStreamInfo{id: "hello", observed: []frame.Tag{frame.Tag(6)}, clientType: ClientTypeSource}
		got := findFunc(source)
		assert.False(t, got)
	})
}

type mockStreamInfo struct {
	name       string
	id         string
	clientType ClientType
	metadata   metadata.M
	observed   []frame.Tag
}

func (s *mockStreamInfo) ID() string                   { return s.id }
func (s *mockStreamInfo) Name() string                 { return s.name }
func (s *mockStreamInfo) Metadata() metadata.M         { return s.metadata }
func (s *mockStreamInfo) ClientType() ClientType       { return s.clientType }
func (s *mockStreamInfo) ObserveDataTags() []frame.Tag { return s.observed }
