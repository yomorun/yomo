package golang

import "testing"

func TestContainsInitWithoutComment(t *testing.T) {
	type args struct {
		source []byte
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "init function with comment",
			args: args{
				source: []byte(`// func Init() error {`),
			},
			want: false,
		},
		{
			name: "init function without comment",
			args: args{
				source: []byte(`func Init() error {`),
			},
			want: true,
		},
		{
			name: "no init function",
			args: args{
				source: []byte(``),
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := containsStringWithoutComment(tt.args.source, "Init()"); got != tt.want {
				t.Errorf("containInitFunc() = %v, want %v", got, tt.want)
			}
		})
	}
}
