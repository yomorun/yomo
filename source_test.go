package yomo

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/core/ylog"
)

func TestSource(t *testing.T) {
	t.Parallel()

	source := NewSource(
		"test-source",
		"localhost:9000",
		WithCredential("token:<CREDENTIAL>"),
		WithLogger(ylog.Default()),
		WithObserveDataTags(0x22),
		WithSourceQuicConfig(core.DefalutQuicConfig),
		WithSourceTLSConfig(nil),
	)

	exit := make(chan struct{})
	time.AfterFunc(time.Second, func() {
		source.Close()
		close(exit)
	})

	source.SetErrorHandler(func(err error) {})

	source.SetReceiveHandler(func(tag frame.Tag, data []byte) {
		assert.Equal(t, 0x22, tag)
		assert.Equal(t, []byte("backflow"), data)
	})

	// connect to zipper
	err := source.Connect()
	assert.Nil(t, err)

	// send data to zipper
	err = source.Write(0x21, []byte("test"))
	assert.Nil(t, err)

	// broadcast data to zipper
	err = source.Broadcast(0x21, []byte("test"))
	assert.Nil(t, err)

	<-exit
}

func TestBuildDadaFrame(t *testing.T) {
	type args struct {
		clientID  string
		broadcast bool
		tid       string
		tag       uint32
		data      []byte
	}
	tests := []struct {
		name   string
		args   args
		want   *frame.DataFrame
		wantmd metadata.M
	}{
		{
			name: "Write",
			args: args{
				clientID:  "aaaaa",
				broadcast: false,
				tid:       "bbbbb",
				tag:       0x21,
				data:      []byte("hello"),
			},
			want: &frame.DataFrame{
				Tag:     0x21,
				Payload: []byte("hello"),
			},
			wantmd: core.NewDefaultMetadata("aaaaa", false, "bbbbb"),
		},
		{
			name: "Broadcast",
			args: args{
				clientID:  "ccccc",
				broadcast: true,
				tid:       "ddddd",
				tag:       0x22,
				data:      []byte("yomo"),
			},
			want: &frame.DataFrame{
				Tag:     0x22,
				Payload: []byte("yomo"),
			},
			wantmd: core.NewDefaultMetadata("ccccc", true, "ddddd"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildDadaFrame(tt.args.clientID, tt.args.broadcast, tt.args.tid, tt.args.tag, tt.args.data)
			gotmd, _ := metadata.Decode(got.Metadata)

			assert.NoError(t, err)
			assert.Equal(t, tt.want.Tag, got.Tag)
			assert.Equal(t, tt.wantmd, gotmd)
			assert.Equal(t, tt.want.Payload, got.Payload)

		})
	}
}
