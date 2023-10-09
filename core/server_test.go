package core

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
	_ "github.com/yomorun/yomo/pkg/auth"
)

func TestMakeSourceTagFindConnectionFunc(t *testing.T) {
	findFunc := sourceIDTagFindConnectionFunc("hello", frame.Tag(7))

	t.Run("find successful", func(t *testing.T) {
		source := &mockConnectionInfo{id: "hello", observed: []frame.Tag{frame.Tag(7)}, clientType: ClientTypeSource}
		got := findFunc(source)
		assert.True(t, got)
	})

	t.Run("find in name failed", func(t *testing.T) {
		source := &mockConnectionInfo{id: "olleh", observed: []frame.Tag{frame.Tag(7)}, clientType: ClientTypeSource}
		got := findFunc(source)
		assert.False(t, got)
	})

	t.Run("find in tag failed", func(t *testing.T) {
		source := &mockConnectionInfo{id: "hello", observed: []frame.Tag{frame.Tag(6)}, clientType: ClientTypeSource}
		got := findFunc(source)
		assert.False(t, got)
	})
}

type mockConnectionInfo struct {
	name       string
	id         string
	clientType ClientType
	metadata   metadata.M
	observed   []frame.Tag
}

func (s *mockConnectionInfo) ID() string                   { return s.id }
func (s *mockConnectionInfo) Name() string                 { return s.name }
func (s *mockConnectionInfo) Metadata() metadata.M         { return s.metadata }
func (s *mockConnectionInfo) ClientType() ClientType       { return s.clientType }
func (s *mockConnectionInfo) ObserveDataTags() []frame.Tag { return s.observed }

func Test_negotiateVersion(t *testing.T) {
	type args struct {
		cVersion string
		sVersion string
	}
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name: "ok",
			args: args{
				cVersion: "1.16.3",
				sVersion: "1.16.3",
			},
			wantErr: nil,
		},
		{
			name: "client empty version",
			args: args{
				cVersion: "",
				sVersion: "1.16.3",
			},
			wantErr: errors.New("invalid semantic version, params="),
		},
		{
			name: "not ok",
			args: args{
				cVersion: "1.15.0",
				sVersion: "1.16.3",
			},
			wantErr: errors.New("yomo: version negotiation failed, client=1.15.0, server=1.16.3"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := negotiateVersion(tt.args.cVersion, tt.args.sVersion)
			assert.Equal(t, tt.wantErr, err)
		})
	}
}
