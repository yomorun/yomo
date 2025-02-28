package auth

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
)

// mockAuth implement `Authentication` interface,
// Authenticate returns true if authed is true, false to false.
type mockAuth struct {
	name   string
	authed bool
}

func (auth mockAuth) Init(args ...string) {}

func (auth mockAuth) Authenticate(payload string) (metadata.M, error) {
	if auth.authed {
		return metadata.M{}, nil
	}
	return metadata.M{}, errors.New("mock auth error")
}

func (auth mockAuth) Name() string {
	if auth.name != "" {
		return auth.name
	}
	return "mock"
}

func TestRegister(t *testing.T) {
	mock1 := mockAuth{name: "mock1"}
	mock2 := mockAuth{name: "mock2"}

	// this does nothing
	Register(nil)
	RegisterAsDefault(nil)

	Register(mock1)
	RegisterAsDefault(mock2)

	actual, ok := GetAuth("mock1")
	assert.Equal(t, mock1, actual)
	assert.True(t, ok)

	actual = DefaultAuth()
	assert.Equal(t, mock2, actual)
}

func TestAuthenticate(t *testing.T) {
	type args struct {
		auths       map[string]Authentication
		defaultAuth Authentication
		hf          *frame.HandshakeFrame
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "auths is nil",
			args: args{
				auths:       nil,
				defaultAuth: nil,
				hf:          &frame.HandshakeFrame{AuthName: "mock", AuthPayload: "mock_payload"},
			},
			want: true,
		},
		{
			name: "hf is nil",
			args: args{
				auths:       map[string]Authentication{"mock": mockAuth{authed: true}},
				defaultAuth: nil,
				hf:          nil,
			},
			want: false,
		},
		{
			name: "hf.AuthName not found",
			args: args{
				auths:       map[string]Authentication{"mock": mockAuth{authed: true}},
				defaultAuth: nil,
				hf:          &frame.HandshakeFrame{AuthName: "mock_not_match", AuthPayload: "mock_payload"},
			},
			want: false,
		},
		{
			name: "auth success",
			args: args{
				auths:       map[string]Authentication{"mock": mockAuth{authed: true}},
				defaultAuth: nil,
				hf:          &frame.HandshakeFrame{AuthName: "mock", AuthPayload: "mock_payload"},
			},
			want: true,
		},
		{
			name: "auth failed",
			args: args{
				auths:       map[string]Authentication{"mock": mockAuth{authed: false}},
				defaultAuth: nil,
				hf:          &frame.HandshakeFrame{AuthName: "mock", AuthPayload: "mock_payload"},
			},
			want: false,
		},
		{
			name: "auth with default",
			args: args{
				auths:       map[string]Authentication{"mock": mockAuth{authed: true}},
				defaultAuth: mockAuth{authed: false},
				hf:          &frame.HandshakeFrame{AuthName: "", AuthPayload: "mock_payload"},
			},
			want: false,
		},
		{
			name: "auth without default",
			args: args{
				auths:       map[string]Authentication{"mock": mockAuth{authed: true}},
				defaultAuth: nil,
				hf:          &frame.HandshakeFrame{AuthName: "", AuthPayload: "mock_payload"},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Authenticate(tt.args.auths, tt.args.defaultAuth, tt.args.hf)
			assert.Equal(t, tt.want, err == nil)
		})
	}
}

func TestNewCredential(t *testing.T) {
	type args struct {
		payload string
	}
	tests := []struct {
		name string
		args args
		want *Credential
	}{
		{
			name: "key value pair",
			args: args{
				payload: "token:the-token",
			},
			want: &Credential{
				name:    "token",
				payload: "the-token",
			},
		},
		{
			name: "not key value pair",
			args: args{
				payload: "abcdefg",
			},
			want: &Credential{
				name:    "",
				payload: "abcdefg",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewCredential(tt.args.payload)

			assert.Equal(t, tt.want.Name(), got.Name())
			assert.Equal(t, tt.want.Payload(), got.Payload())
		})
	}
}
