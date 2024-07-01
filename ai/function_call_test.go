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
	t.Run("ok", func(t *testing.T) {
		bytes, err := original.Bytes()
		assert.NoError(t, err)

		actual := &FunctionCall{}
		err = actual.FromBytes(bytes)

		assert.NoError(t, err)
		assert.Equal(t, original, actual)
	})

	t.Run("data is not a json string", func(t *testing.T) {
		actual := &FunctionCall{}
		err := actual.FromBytes([]byte("not a json string"))
		assert.EqualError(t, err, "llm-sfn: cannot read function call object from context data")
	})

	t.Run("data cannot be unmarshal as FunctionCall", func(t *testing.T) {
		actual := &FunctionCall{}
		err := actual.FromBytes([]byte(`{"hello":"yomo"}`))
		assert.EqualError(t, err, "llm-sfn: cannot read function call object from context data")
	})
}
