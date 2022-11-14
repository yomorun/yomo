package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core/frame"
)

// mockAuth implement `Authentication` interface,
// Authenticate returns true if authed is true, false to false.
type mockAuth struct{ authed bool }

func (auth mockAuth) Init(args ...string)              {}
func (auth mockAuth) Authenticate(payload string) bool { return auth.authed }
func (auth mockAuth) Name() string                     { return "mock" }

func init() { Register(mockAuth{}) }

func Test_Authenticate(t *testing.T) {
	type args struct {
		auths  map[string]Authentication
		auther Auther
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "auths is nil",
			args: args{
				auths:  nil,
				auther: frame.NewHandshakeFrame("", "", byte(1), []frame.Tag{}, "mock", "mock_payload"),
			},
			want: true,
		},
		{
			name: "auther is nil",
			args: args{
				auths:  map[string]Authentication{"mock": mockAuth{authed: true}},
				auther: nil,
			},
			want: false,
		},
		{
			name: "auther not found",
			args: args{
				auths:  map[string]Authentication{"mock": mockAuth{authed: true}},
				auther: frame.NewHandshakeFrame("", "", byte(1), []frame.Tag{}, "mock_not_match", "mock_payload"),
			},
			want: false,
		},
		{
			name: "auth success",
			args: args{
				auths:  map[string]Authentication{"mock": mockAuth{authed: true}},
				auther: frame.NewHandshakeFrame("", "", byte(1), []frame.Tag{}, "mock", "mock_payload"),
			},
			want: true,
		},
		{
			name: "auth failed",
			args: args{
				auths:  map[string]Authentication{"mock": mockAuth{authed: false}},
				auther: frame.NewHandshakeFrame("", "", byte(1), []frame.Tag{}, "mock", "mock_payload"),
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Authenticate(tt.args.auths, tt.args.auther)
			assert.Equal(t, tt.want, got)
		})
	}
}
