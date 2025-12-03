package caller

import (
	"context"
	"testing"

	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
)

func TestMockCaller(t *testing.T) {

	caller := MockCaller([]MockFunctionCall{
		{ToolID: "call_abc123", FunctionName: "get_current_weather", RespContent: "temperature: 31°C"},
	})

	results, err := caller.Call(context.Background(), "transID", "reqID", []byte("agentContext"), []openai.ToolCall{
		{ID: "call_abc123", Type: openai.ToolTypeFunction, Function: openai.FunctionCall{Name: "get_current_weather"}},
	}, nil)

	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "temperature: 31°C", results[0].Content)
}
