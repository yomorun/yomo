package template

import (
	"testing"
)

func TestGenNameByCommand(t *testing.T) {
	type args struct {
		command string
		sfnType string
		lang    string
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
				lang:    "go",
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
				lang:    "node",
				isTest:  true,
			},
			want:    "node/init_normal_test.tmpl",
			wantErr: false,
		},
		{
			name: "default_command_llm_go",
			args: args{
				command: "",
				sfnType: "llm",
				lang:    "go",
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
				lang:    "go",
				isTest:  false,
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "unsupported_lang",
			args: args{
				command: "init",
				sfnType: "llm",
				lang:    "unsupported",
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
				lang:    "go",
				isTest:  false,
			},
			want:    "go/init_llm.tmpl",
			wantErr: false,
		},
		{
			name: "default_lang",
			args: args{
				command: "init",
				sfnType: "llm",
				lang:    "",
				isTest:  false,
			},
			want:    "go/init_llm.tmpl",
			wantErr: false,
		},
		{
			name: "default_sfnType_and_lang",
			args: args{
				command: "init",
				sfnType: "",
				lang:    "",
				isTest:  false,
			},
			want:    "go/init_llm.tmpl",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := genNameByCommand(tt.args.command, tt.args.sfnType, tt.args.lang, tt.args.isTest)
			if (err != nil) != tt.wantErr {
				t.Errorf("genNameByCommand() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("genNameByCommand() = %v, want %v", got, tt.want)
			}
		})
	}
}
