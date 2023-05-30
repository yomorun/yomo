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
		StreamType:      0x10,
		ObserveDataTags: []uint32{1, 2, 3},
		Metadata:        []byte{'c'},
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
			name: "AuthenticationFrame",
			args: args{
				newF: new(frame.AuthenticationFrame),
				dataF: &frame.AuthenticationFrame{
					AuthName:    "token",
					AuthPayload: "a",
				},
				data: []byte{
					0x80 | byte(frame.TypeAuthenticationFrame), 0xa,
					byte(tagAuthenticationName), 0x05, 0x74, 0x6f, 0x6b, 0x65, 0x6e,
					byte(tagAuthenticationPayload), 0x01, 0x61,
				},
			},
		},
		{
			name: "AuthenticationAckFrame",
			args: args{
				newF:  new(frame.AuthenticationAckFrame),
				dataF: &frame.AuthenticationAckFrame{},
				data:  []byte{0x91, 0x0},
			},
		},
		{
			name: "BackflowFrame",
			args: args{
				newF:  new(frame.BackflowFrame),
				dataF: &frame.BackflowFrame{Tag: 0x10, Carriage: []byte("hello backflow")},
				data: []byte{0xad, 0x13, 0x1, 0x1, 0x10, 0x2, 0xe, 0x68, 0x65, 0x6c,
					0x6c, 0x6f, 0x20, 0x62, 0x61, 0x63, 0x6b, 0x66, 0x6c, 0x6f, 0x77},
			},
		},
		{
			name: "DataFrame",
			args: args{
				newF: new(frame.DataFrame),
				dataF: &frame.DataFrame{
					Meta: &frame.MetaFrame{
						TID: "aaaaa", Broadcast: true, Metadata: []byte("bbbb"),
					},
					Payload: &frame.PayloadFrame{
						Tag:      0x15,
						Carriage: []byte("yomo"),
					},
				},
				data: []byte{0xbf, 0x1f, 0xaf, 0x12, 0x1, 0x5, 0x61, 0x61, 0x61, 0x61, 0x61,
					0x2, 0x0, 0x3, 0x4, 0x62, 0x62, 0x62, 0x62, 0x4, 0x1, 0x1, 0xae, 0x9,
					0x1, 0x1, 0x15, 0x2, 0x4, 0x79, 0x6f, 0x6d, 0x6f},
			},
		},
		{
			name: "HandshakeAckFrame",
			args: args{
				newF:  new(frame.HandshakeAckFrame),
				dataF: &frame.HandshakeAckFrame{StreamID: "mock-stream-id"},
				data: []byte{0xa9, 0x10, 0x28, 0xe, 0x6d, 0x6f, 0x63, 0x6b, 0x2d, 0x73, 0x74,
					0x72, 0x65, 0x61, 0x6d, 0x2d, 0x69, 0x64},
			},
		},
		{
			name: "HandshakeFrame",
			args: args{
				newF: new(frame.HandshakeFrame),
				dataF: &frame.HandshakeFrame{
					Name:            "the-name",
					ID:              "the-id",
					StreamType:      104,
					ObserveDataTags: []uint32{'a', 'b', 'c'},
					Metadata:        []byte{'d', 'e', 'f'},
				},
				data: []byte{0xb1, 0x28, 0x1, 0x8, 0x74, 0x68, 0x65, 0x2d, 0x6e, 0x61, 0x6d,
					0x65, 0x3, 0x6, 0x74, 0x68, 0x65, 0x2d, 0x69, 0x64, 0x2, 0x1, 0x68, 0x6, 0xc,
					0x61, 0x0, 0x0, 0x0, 0x62, 0x0, 0x0, 0x0, 0x63, 0x0, 0x0, 0x0, 0x7, 0x3, 0x64,
					0x65, 0x66},
			},
		},
		{
			name: "HandshakeRejectedFrame",
			args: args{
				newF: new(frame.HandshakeRejectedFrame),
				dataF: &frame.HandshakeRejectedFrame{
					ID:      "hello",
					Message: "yomo",
				},
				data: []byte{0x94, 0xd, 0x15, 0x5, 0x68, 0x65, 0x6c, 0x6c, 0x6f, 0x16, 0x4, 0x79, 0x6f,
					0x6d, 0x6f},
			},
		},
		{
			name: "RejectedFrame",
			args: args{
				newF: new(frame.RejectedFrame),
				dataF: &frame.RejectedFrame{
					Code:    123,
					Message: "rejected error",
				},
				data: []byte{0xb9, 0x13, 0x1, 0x1, 0x7b, 0x2, 0xe, 0x72, 0x65, 0x6a, 0x65, 0x63, 0x74,
					0x65, 0x64, 0x20, 0x65, 0x72, 0x72, 0x6f, 0x72},
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
