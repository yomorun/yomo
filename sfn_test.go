package yomo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSfnConnectToServer(t *testing.T) {
	sfn := NewStreamFunction(
		"test-sfn",
		WithZipperAddr("localhost:9000"),
		WithObserveDataTags(0x33),
	)
	defer sfn.Close()

	// set handler
	sfn.SetHandler(nil)

	// connect to server
	err := sfn.Connect()
	assert.Nil(t, err)
}
