package version

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParse(t *testing.T) {
	type args struct {
		str string
	}
	tests := []struct {
		name    string
		args    args
		want    *Version
		wantErr error
	}{
		{
			name: "empty",
			args: args{
				str: "",
			},
			want:    nil,
			wantErr: errors.New("empty version string"),
		},
		{
			name: "ok",
			args: args{
				str: "1.16.3",
			},
			want: &Version{Major: 1, Minor: 16, Patch: 3},
		},
		{
			name: "invalid semantic version",
			args: args{
				str: "1.16.3-beta.1",
			},
			want:    nil,
			wantErr: errors.New("invalid semantic version, params=1.16.3-beta.1"),
		},
		{
			name: "invalid version major",
			args: args{
				str: "xx.16.3",
			},
			want:    nil,
			wantErr: errors.New("invalid version major, params=xx.16.3"),
		},
		{
			name: "invalid version minor",
			args: args{
				str: "1.yy.3",
			},
			want:    nil,
			wantErr: errors.New("invalid version minor, params=1.yy.3"),
		},
		{
			name: "invalid version patch",
			args: args{
				str: "1.16.3-beta",
			},
			want:    nil,
			wantErr: errors.New("invalid version patch, params=1.16.3-beta"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := Parse(tt.args.str)
			assert.Equal(t, tt.wantErr, gotErr)
			assert.Equal(t, tt.want, got)
		})
	}
}
