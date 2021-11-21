package yomo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSfnConnectToServer(t *testing.T) {
	sfn := NewStreamFunction("test-sfn", WithZipperAddr("localhost:9000"))
	defer sfn.Close()

	// set only monitoring data which tag=0x33
	sfn.SetObserveDataTag(0x33)

	// set handler
	sfn.SetHandler(nil)

	// connect to server
	err := sfn.Connect()
	assert.Nil(t, err)
}
