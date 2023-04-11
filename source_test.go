package yomo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSourceSendDataToServer(t *testing.T) {
	source := NewSource("test-source", "localhost:9000")
	defer source.Close()

	// connect to server
	err := source.Connect()
	assert.Nil(t, err)

	// send data to server
	n, err := source.Write([]byte("test"))
	assert.Greater(t, n, 0, "[source.Write] expected n > 0, but got %d", n)
	assert.Nil(t, err)
}
