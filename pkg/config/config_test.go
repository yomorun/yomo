package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseConfigFile(t *testing.T) {
	t.Run("ext incorrect", func(t *testing.T) {
		_, err := ParseConfigFile(filepath.Join(t.TempDir(), "zipper.yoml"))
		assert.Equal(t, ErrConfigExt, err)
	})
	t.Run("file not exist", func(t *testing.T) {
		_, err := ParseConfigFile(filepath.Join(t.TempDir(), "config.yaml"))
		assert.Error(t, err)
	})
	t.Run("normal", func(t *testing.T) {
		conf, err := ParseConfigFile("../../test/config.yaml")
		assert.NoError(t, err)

		assert.Equal(t, "america", conf.Name)
		assert.Equal(t, "0.0.0.0", conf.Host)
		assert.Equal(t, 9000, conf.Port)
	})
}

func TestValidateConfig(t *testing.T) {
	type args struct {
		conf *Config
	}
	tests := []struct {
		name          string
		args          args
		wantErrString string
	}{
		{
			name: "name empty",
			args: args{
				conf: &Config{},
			},
			wantErrString: "config: the name is required",
		},
		{
			name: "host empty",
			args: args{
				conf: &Config{
					Name: "name",
				},
			},
			wantErrString: "config: the host is required",
		},
		{
			name: "port empty",
			args: args{
				conf: &Config{
					Name: "name",
					Host: "0.0.0.0",
				},
			},
			wantErrString: "config: the port is required",
		},
		{
			name: "functions empty",
			args: args{
				conf: &Config{
					Name: "name",
					Host: "0.0.0.0",
					Port: 9000,
				},
			},
			wantErrString: "config: the functions cannot be an empty",
		},
		{
			name: "functions lack name",
			args: args{
				conf: &Config{
					Name:      "name",
					Host:      "0.0.0.0",
					Port:      9000,
					Functions: []Function{{}},
				},
			},
			wantErrString: "config: the functions must have the name field",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.args.conf)
			assert.Equal(t, tt.wantErrString, err.Error())
		})
	}
}
