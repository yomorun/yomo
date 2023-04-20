package yomo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSfnConnectToServer(t *testing.T) {
	sfn := NewStreamFunction(
		"test-sfn",
		"localhost:9000",
	)
	sfn.SetObserveDataTags(0x33)
	defer sfn.Close()

	// set handler
	sfn.SetHandler(nil)

	// connect to server
	err := sfn.Connect()
	assert.Nil(t, err)
}
