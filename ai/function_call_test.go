package ai

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

var jsonStr = "{\"req_id\":\"yYdzyl\",\"arguments\":\"{\\n  \\\"sourceTimezone\\\": \\\"America/Los_Angeles\\\",\\n  \\\"targetTimezone\\\": \\\"Asia/Singapore\\\",\\n  \\\"timeString\\\": \\\"2024-03-25 07:00:00\\\"\\n}\",\"tool_call_id\":\"call_aZrtm5xcLs1qtP0SWo4CZi75\",\"function_name\":\"fn-timezone-converter\",\"is_ok\":false}"

var jsonStrWithResult = func(result string) string {
	return fmt.Sprintf("{\"req_id\":\"yYdzyl\",\"result\":\"%s\",\"arguments\":\"{\\n  \\\"sourceTimezone\\\": \\\"America/Los_Angeles\\\",\\n  \\\"targetTimezone\\\": \\\"Asia/Singapore\\\",\\n  \\\"timeString\\\": \\\"2024-03-25 07:00:00\\\"\\n}\",\"tool_call_id\":\"call_aZrtm5xcLs1qtP0SWo4CZi75\",\"function_name\":\"fn-timezone-converter\",\"is_ok\":true}", result)
}

var jsonStrWithError = func(err string) string {
	return fmt.Sprintf("{\"req_id\":\"yYdzyl\",\"arguments\":\"{\\n  \\\"sourceTimezone\\\": \\\"America/Los_Angeles\\\",\\n  \\\"targetTimezone\\\": \\\"Asia/Singapore\\\",\\n  \\\"timeString\\\": \\\"2024-03-25 07:00:00\\\"\\n}\",\"tool_call_id\":\"call_aZrtm5xcLs1qtP0SWo4CZi75\",\"function_name\":\"fn-timezone-converter\",\"is_ok\":true,\"error\":\"%s\"}", err)
}

var errJSONStr = "{a}"

var original = &FunctionCall{
	ReqID:        "yYdzyl",
	Arguments:    "{\n  \"sourceTimezone\": \"America/Los_Angeles\",\n  \"targetTimezone\": \"Asia/Singapore\",\n  \"timeString\": \"2024-03-25 07:00:00\"\n}",
	FunctionName: "fn-timezone-converter",
	ToolCallID:   "call_aZrtm5xcLs1qtP0SWo4CZi75",
	IsOK:         false,
}

func TestFunctionCallBytes(t *testing.T) {
	// Marshal the FunctionCall into bytes
	bytes, err := original.Bytes()
	// assert.NoError(t, err)

	// // Unmarshal the bytes into a new FunctionCall
	// target := &FunctionCall{}
	// err = target.fromBytes(bytes)

	assert.NoError(t, err)
	assert.Equal(t, string(bytes), jsonStr, "Original and bytes should be equal")
}

func TestReadFunctionCall(t *testing.T) {
	t.Run("ctx.Data is nil", func(t *testing.T) {
		ctx := NewMockContext(nil, 0)
		fnCall := &FunctionCall{}
		err := ctx.ReadLLMFunctionCall(fnCall)
		assert.Error(t, err)
	})

	t.Run("ctx.Data is invalid", func(t *testing.T) {
		ctx := NewMockContext([]byte(errJSONStr), 0)
		fnCall := &FunctionCall{}
		err := ctx.ReadLLMFunctionCall(&fnCall)
		assert.Error(t, err)
	})
}

func TestReadLLMArguments(t *testing.T) {
	ctx := NewMockContext([]byte(jsonStr), 0x10)
	target := make(map[string]string)
	err := ctx.ReadLLMArguments(&target)

	assert.NoError(t, err)
	assert.Equal(t, "America/Los_Angeles", target["sourceTimezone"])
	assert.Equal(t, "Asia/Singapore", target["targetTimezone"])
	assert.Equal(t, "2024-03-25 07:00:00", target["timeString"])
}

func TestWriteLLMResult(t *testing.T) {
	ctx := NewMockContext([]byte(jsonStr), 0x10)

	// read
	target := make(map[string]string)
	err := ctx.ReadLLMArguments(&target)
	assert.NoError(t, err)

	// write
	err = ctx.WriteLLMResult("test result")
	assert.NoError(t, err)

	res := ctx.RecordsWritten()
	assert.Equal(t, ReducerTag, res[0].Tag)
	assert.Equal(t, jsonStrWithResult("test result"), string(res[0].Data))
}
