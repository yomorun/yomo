package frame

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBackflowFrameEncode(t *testing.T) {
	var (
		tag      = Tag(22)
		carriage = []byte("hello backflow")
	)
	f := NewBackflowFrame(tag, []byte{})

	f.SetCarriage(carriage)

	assert.Equal(t, TagOfBackflowFrame, f.Type())
	assert.Equal(t, f.GetCarriage(), carriage)
	assert.Equal(t, f.GetDataTag(), tag)
	assert.Equal(t, []byte{0xad, 0x13, 0x1, 0x1, 0x16, 0x2, 0xe, 0x68, 0x65, 0x6c, 0x6c, 0x6f, 0x20, 0x62, 0x61, 0x63, 0x6b, 0x66, 0x6c, 0x6f, 0x77}, f.Encode())
}

func TestBackflowFrameDecode(t *testing.T) {
	f := NewBackflowFrame(Tag(22), []byte("hello backflow"))

	buf := f.Encode()

	df, err := DecodeToBackflowFrame(buf)

	assert.NoError(t, err)
	assert.Equal(t, df, f)
}
