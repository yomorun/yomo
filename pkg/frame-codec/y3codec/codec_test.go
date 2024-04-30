package y3codec

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	frame "github.com/yomorun/yomo/core/frame"
)

func TestReadPacket(t *testing.T) {
	prw := PacketReadWriter()
	codec := Codec()

	hf := &frame.HandshakeFrame{
		Name:            "a",
		ID:              "b",
		ClientType:      0x10,
		ObserveDataTags: []uint32{1, 2, 3},
	}
	b, err := codec.Encode(hf)
	assert.NoError(t, err)

	stream := bytes.NewBuffer(b)

	ft, bb, err := prw.ReadPacket(stream)
	assert.NoError(t, err)
	assert.Equal(t, b, bb)
	assert.Equal(t, frame.TypeHandshakeFrame, ft)

	ft, bb, err = prw.ReadPacket(stream)
	assert.Equal(t, io.EOF, err)
	assert.Equal(t, []byte(nil), bb)
	assert.Equal(t, frame.Type(0x0), ft)

	err = prw.WritePacket(stream, frame.TypeHandshakeFrame, nil)
	assert.NoError(t, err)
}

func TestCodec(t *testing.T) {
	type args struct {
		newF      frame.Frame
		dataF     frame.Frame
		data      []byte
		encodeErr error
		decodeErr error
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "DataFrame",
			args: args{
				newF: new(frame.DataFrame),
				dataF: &frame.DataFrame{
					Tag:      0x15,
					Metadata: []byte("metadata"),
					Payload:  []byte("yomo"),
				},
				data: []byte{
					0xbf, 0x13, 0x1, 0x1, 0x15, 0x3, 0x8, 0x6d, 0x65, 0x74,
					0x61, 0x64, 0x61, 0x74, 0x61, 0x2, 0x4, 0x79, 0x6f, 0x6d, 0x6f,
				},
			},
		},
		{
			name: "HandshakeFrame",
			args: args{
				newF: new(frame.HandshakeFrame),
				dataF: &frame.HandshakeFrame{
					Name:               "the-name",
					ID:                 "the-id",
					ClientType:         104,
					ObserveDataTags:    []uint32{'a', 'b', 'c'},
					AuthName:           "ddddd",
					AuthPayload:        "eeeee",
					Version:            "2014-01-03",
					FunctionDefinition: []byte("handshake-metadata"),
					WantedTarget:       "the-wanted-target",
				},
				data: []byte{0xb1, 0x80, 0x64, 0x1, 0x8, 0x74, 0x68, 0x65, 0x2d,
					0x6e, 0x61, 0x6d, 0x65, 0x3, 0x6, 0x74, 0x68, 0x65, 0x2d, 0x69,
					0x64, 0x2, 0x1, 0x68, 0x6, 0xc, 0x61, 0x0, 0x0, 0x0, 0x62, 0x0,
					0x0, 0x0, 0x63, 0x0, 0x0, 0x0, 0x4, 0x5, 0x64, 0x64, 0x64, 0x64,
					0x64, 0x5, 0x5, 0x65, 0x65, 0x65, 0x65,
					0x65, 0x7, 0xa, 0x32, 0x30, 0x31, 0x34, 0x2d, 0x30, 0x31, 0x2d,
					0x30, 0x33, 0x9, 0x12, 0x68, 0x61, 0x6e, 0x64, 0x73, 0x68, 0x61,
					0x6b, 0x65, 0x2d, 0x6d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61,
					0x8, 0x11, 0x74, 0x68, 0x65, 0x2d, 0x77, 0x61, 0x6e, 0x74, 0x65,
					0x64, 0x2d, 0x74, 0x61, 0x72, 0x67, 0x65, 0x74},
			},
		},
		{
			name: "HandshakeAckFrame",
			args: args{
				newF:  new(frame.HandshakeAckFrame),
				dataF: &frame.HandshakeAckFrame{},
				data:  []byte{0xa9, 0x0},
			},
		},
		{
			name: "RejectedFrame",
			args: args{
				newF: new(frame.RejectedFrame),
				dataF: &frame.RejectedFrame{
					Message: "rejected error",
				},
				data: []byte{
					0xb9, 0x10, 0x1, 0xe, 0x72, 0x65, 0x6a, 0x65, 0x63, 0x74, 0x65,
					0x64, 0x20, 0x65, 0x72, 0x72, 0x6f, 0x72,
				},
			},
		},
		{
			name: "GoawayFrame",
			args: args{
				newF: new(frame.GoawayFrame),
				dataF: &frame.GoawayFrame{
					Message: "goaway error",
				},
				data: []byte{
					0xae, 0xe, 0x1, 0xc, 0x67, 0x6f, 0x61, 0x77, 0x61, 0x79, 0x20,
					0x65, 0x72, 0x72, 0x6f, 0x72,
				},
			},
		},
		{
			name: "ConnectToFrame",
			args: args{
				newF: new(frame.ConnectToFrame),
				dataF: &frame.ConnectToFrame{
					Endpoint: "11.11.11.11:8080",
				},
				data: []byte{
					0xbe, 0x12, 0x1, 0x10, 0x31, 0x31, 0x2e, 0x31, 0x31, 0x2e,
					0x31, 0x31, 0x2e, 0x31, 0x31, 0x3a, 0x38, 0x30, 0x38, 0x30,
				},
			},
		},
		{
			name: "error",
			args: args{
				newF:      nil,
				dataF:     nil,
				data:      []byte(nil),
				encodeErr: ErrUnknownFrame,
				decodeErr: ErrUnknownFrame,
			},
		},
	}
	for _, tt := range tests {
		codec := Codec()
		t.Run(tt.name, func(t *testing.T) {
			t.Run("Encode", func(t *testing.T) {
				got, err := codec.Encode(tt.args.dataF)
				assert.Equal(t, tt.args.encodeErr, err)
				assert.Equal(t, tt.args.data, got)
			})
			t.Run("Decode", func(t *testing.T) {
				err := codec.Decode(tt.args.data, tt.args.newF)
				assert.Equal(t, tt.args.decodeErr, err)
				assert.EqualValues(t, tt.args.dataF, tt.args.newF)
			})
		})
	}
}
