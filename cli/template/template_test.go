package template

import (
	"testing"
)

func TestGetTemplateFileName(t *testing.T) {
	type args struct {
		command string
		sfnType string
		runtime string
		isTest  bool
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "init_llm_go",
			args: args{
				command: "init",
				sfnType: "llm",
				runtime: "go",
				isTest:  false,
			},
			want:    "go/init_llm.tmpl",
			wantErr: false,
		},
		{
			name: "init_normal_node_test",
			args: args{
				command: "init",
				sfnType: "normal",
				runtime: "node",
				isTest:  true,
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "default_command_llm_go",
			args: args{
				command: "",
				sfnType: "llm",
				runtime: "go",
				isTest:  false,
			},
			want:    "go/init_llm.tmpl",
			wantErr: false,
		},
		{
			name: "unsupported_sfnType",
			args: args{
				command: "init",
				sfnType: "unsupported",
				runtime: "go",
				isTest:  false,
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "unsupported_runtime",
			args: args{
				command: "init",
				sfnType: "llm",
				runtime: "unsupported",
				isTest:  false,
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "default_sfnType",
			args: args{
				command: "init",
				sfnType: "",
				runtime: "go",
				isTest:  false,
			},
			want:    "go/init_llm.tmpl",
			wantErr: false,
		},
		{
			name: "default_runtime",
			args: args{
				command: "init",
				sfnType: "llm",
				runtime: "",
				isTest:  false,
			},
			want:    "node/init_llm.tmpl",
			wantErr: false,
		},
		{
			name: "default_sfnType_and_runtime",
			args: args{
				command: "init",
				sfnType: "",
				runtime: "",
				isTest:  false,
			},
			want:    "node/init_llm.tmpl",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getTemplateFileName(tt.args.command, tt.args.sfnType, tt.args.runtime, tt.args.isTest)
			if (err != nil) != tt.wantErr {
				t.Errorf("getTemplateFileName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("getTemplateFileName() = %v, want %v", got, tt.want)
			}
		})
	}
}
