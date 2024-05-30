package ai

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var original = &FunctionCall{
	ReqID:        "yYdzyl",
	Arguments:    "{\n  \"sourceTimezone\": \"America/Los_Angeles\",\n  \"targetTimezone\": \"Asia/Singapore\",\n  \"timeString\": \"2024-03-25 07:00:00\"\n}",
	FunctionName: "fn-timezone-converter",
	ToolCallID:   "call_aZrtm5xcLs1qtP0SWo4CZi75",
	IsOK:         false,
}

func TestFunctionCallBytes(t *testing.T) {
	bytes, err := original.Bytes()
	assert.NoError(t, err)

	actual := &FunctionCall{}
	err = actual.FromBytes(bytes)

	assert.NoError(t, err)
	assert.Equal(t, original, actual)
}
