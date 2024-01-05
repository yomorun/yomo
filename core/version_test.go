package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core/frame"
)

func TestNegotiateVersion(t *testing.T) {
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
				cVersion: "2024-01-03",
				sVersion: "2024-01-03",
			},
			wantErr: nil,
		},
		{
			name: "not ok",
			args: args{
				cVersion: "2024-01-03",
				sVersion: "2024-02-03",
			},
			wantErr: &ErrRejected{Message: "version negotiation failed: client=2024-01-03, server=2024-02-03"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := DefaultVersionNegotiateFunc(tt.args.cVersion, tt.args.sVersion)
			assert.Equal(t, tt.wantErr, err)
		})
	}
}

type mockFrameWriter struct {
	f frame.Frame
}

func (m *mockFrameWriter) WriteFrame(f frame.Frame) error {
	m.f = f
	return nil
}
