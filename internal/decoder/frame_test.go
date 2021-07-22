package decoder

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadBytesFromFrameDecoder(t *testing.T) {
	t.Run("Read raw bytes from frame decoder", func(t *testing.T) {
		b := &bytes.Buffer{}
		// first 3 bytes indicates the length of frame is 3.
		// the bytes are full matched.
		b.Write([]byte{0, 0, 4, 0, 1, 2, 3})

		decoder := NewFrameDecoder(b)
		actual, err := decoder.Read(true)
		expected := []byte{0, 1, 2, 3}
		assert.NoError(t, err)
		assert.Equal(t, expected, actual)

		b.Reset()
		// more bytes, only read first 3 bytes.
		b.Write([]byte{0, 0, 4, 0, 1, 2, 3, 4, 5})
		decoder = NewFrameDecoder(b)
		actual, err = decoder.Read(true)
		expected = []byte{0, 1, 2, 3}
		assert.NoError(t, err)
		assert.Equal(t, expected, actual)

		b.Reset()
		// separated bytes, combine them into one bytes.
		b.Write([]byte{0, 0, 6, 0, 1, 2, 3})
		b.Write([]byte{4, 5})
		decoder = NewFrameDecoder(b)
		actual, err = decoder.Read(true)
		expected = []byte{0, 1, 2, 3, 4, 5}
		assert.NoError(t, err)
		assert.Equal(t, expected, actual)
	})

	t.Run("Read frame bytes from frame decoder", func(t *testing.T) {
		b := &bytes.Buffer{}
		// first 3 bytes indicates the length of frame is 3.
		b.Write([]byte{0, 0, 4, 0, 1, 2, 3, 5, 6})

		decoder := NewFrameDecoder(b)
		actual, err := decoder.Read(false)
		expected := []byte{0, 0, 4, 0, 1, 2, 3}
		assert.NoError(t, err)
		assert.Equal(t, expected, actual)
	})
}

func TestReadBrokenFromFrameDecoder(t *testing.T) {
	b := &bytes.Buffer{}
	// not enough length frame
	b.Write([]byte{0, 0})
	decoder := NewFrameDecoder(b)
	_, err := decoder.Read(true)
	assert.Equal(t, io.EOF, err)

	// first 3 bytes indicates the length of frame is 5.
	// not enough payload, it only has 3 bytes.
	b.Write([]byte{0, 0, 6, 0, 1, 2, 3})
	decoder = NewFrameDecoder(b)
	_, err = decoder.Read(true)
	assert.Equal(t, io.EOF, err)

	// empty length
	b.Write([]byte{0, 0, 0, 0, 0, 0, 0})
	decoder = NewFrameDecoder(b)
	_, err = decoder.Read(true)
	assert.Equal(t, io.EOF, err)
}
