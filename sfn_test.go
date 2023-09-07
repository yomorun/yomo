package yomo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSfnConnectToServer(t *testing.T) {
	sfn := NewStreamFunction(
		"sfn-ai-stream-response",
		"localhost:9000",
		WithSfnCredential("token:<CREDENTIAL>"),
	)
	sfn.SetObserveDataTags(0x33)
	defer sfn.Close()

	// set handler
	sfn.SetHandler(nil)

	// connect to server
	err := sfn.Connect()
	assert.Nil(t, err)
}

func TestSfnInit(t *testing.T) {
	sfn := NewStreamFunction(
		"test-sfn",
		"localhost:9000",
	)
	var total int64
	err := sfn.Init(func() error {
		total++
		return nil
	})
	assert.Nil(t, err)
	assert.Equal(t, int64(1), total)
}
