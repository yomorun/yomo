package main

import (
	"reflect"
	"testing"

	"github.com/yomorun/yomo/serverless/mock"
)

func TestHandler(t *testing.T) {
	tests := []struct {
		name string
		ctx  *mock.MockContext
		// want is the expected data and tag that be written by ctx.Write
		want []mock.WriteRecord
	}{
		{
			name: "upper",
			ctx:  mock.NewMockContext([]byte("hello"), 0x33),
			want: []mock.WriteRecord{
				{Data: []byte("HELLO"), Tag: 0x34},
			},
		},
		// TODO: add more test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Handler(tt.ctx)
			got := tt.ctx.RecordsWritten()

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("TestHandler got: %v, want: %v", got, tt.want)
			}
		})
	}
}
