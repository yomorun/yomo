package core

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	_ "github.com/yomorun/yomo/pkg/auth"
)

func TestRejectHandshake(t *testing.T) {
	type args struct {
		err error
	}
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name: "nil error",
			args: args{
				err: nil,
			},
			wantErr: nil,
		},
		{
			name: "error",
			args: args{
				err: errors.New("some error"),
			},
			wantErr: errors.New("some error"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &mockFrameWriter{}
			err := rejectHandshake(w, tt.args.err)
			assert.Equal(t, err, tt.wantErr)
		})
	}
}

func TestConnectToNewEndpoint(t *testing.T) {
	type args struct {
		err *ErrConnectTo
	}
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name: "nil error",
			args: args{
				err: nil,
			},
			wantErr: nil,
		},
		{
			name: "error",
			args: args{
				err: &ErrConnectTo{Endpoint: "11.11.11.11:8000"},
			},
			wantErr: &ErrConnectTo{Endpoint: "11.11.11.11:8000"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &mockFrameWriter{}
			err := connectToNewEndpoint(w, tt.args.err)
			assert.Equal(t, err, tt.wantErr)
		})
	}
}
