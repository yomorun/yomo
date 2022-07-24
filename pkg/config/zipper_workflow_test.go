package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseWorkflowConfig(t *testing.T) {
	type args struct {
		ext  string
		data string
	}
	tests := []struct {
		name          string
		args          args
		want          *WorkflowConfig
		wantErr       bool
		wantErrString string
	}{
		{
			name: "normal config",
			args: args{
				ext: ".yaml",
				data: `name: Service
host: localhost
port: 9000
functions:
  - name: Noise
  - name: Noise2`,
			},
			want: &WorkflowConfig{
				Name: "Service",
				Host: "localhost",
				Port: 9000,
				Workflow: Workflow{
					Functions: []App{
						{Name: "Noise"},
						{Name: "Noise2"},
					},
				}},
			wantErr:       false,
			wantErrString: "",
		},
		{
			name: "not yaml extension",
			args: args{
				ext:  ".json",
				data: "{}",
			},
			want:          nil,
			wantErr:       true,
			wantErrString: ErrWorkflowConfigExt.Error(),
		},

		{
			name: "not a yaml format",
			args: args{
				ext:  ".yaml",
				data: `abcdefg`,
			},
			want:          nil,
			wantErr:       true,
			wantErrString: "yaml: unmarshal errors:\n  line 1: cannot unmarshal !!str `abcdefg` into config.WorkflowConfig",
		},
		{
			name: "config content error",
			args: args{
				ext:  ".yaml",
				data: `nema: wrong`,
			},
			want:          nil,
			wantErr:       true,
			wantErrString: "Missing name, host or port in workflow config. ",
		},
		{
			name: "missing functions name",
			args: args{
				ext: ".yaml",
				data: `name: Service
host: localhost
port: 9000
functions:
  - name: 
  - name: Noise2`,
			},
			want:          nil,
			wantErr:       true,
			wantErrString: "Missing name, host or port in Functions",
		},
	}
	for _, tt := range tests {
		config := filepath.Join(t.TempDir(), "config"+tt.args.ext)

		if err := os.WriteFile(config, []byte(tt.args.data), 0o666); err != nil {
			t.Error(err)
		}
		defer os.Remove(config)

		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseWorkflowConfig(config)
			if err != nil {
				assert.Equal(t, tt.wantErrString, err.Error())
			}
			if (err != nil) != tt.wantErr {
				assert.Equal(t, tt.wantErr, (err != nil))
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNotExistConfig(t *testing.T) {
	config := filepath.Join(t.TempDir(), "config.yaml")
	os.Remove(config)

	_, err := LoadWorkflowConfig(config)

	assert.Equal(t, fmt.Sprintf("open %s: no such file or directory", config), err.Error())
}

func TestValidateNil(t *testing.T) {
	assert.Equal(t, "workflow: config nil", validateWorkflowConfig(nil).Error())
}
