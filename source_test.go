package yomo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSourceSendDataToServer(t *testing.T) {
	source := NewSource("test-source", "localhost:9000", WithCredential("token:<CREDENTIAL>"))
	defer source.Close()

	// connect to server
	err := source.Connect()
	assert.Nil(t, err)

	// send data to server
	err = source.Write(0x21, []byte("test"))
	assert.Nil(t, err)
}
