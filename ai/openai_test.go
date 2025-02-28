package ai

import (
	"testing"

	"github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
)

func TestConvertToInvokeResponse(t *testing.T) {
	type args struct {
		res   *openai.ChatCompletionResponse
		tools []openai.Tool
	}
	tests := []struct {
		name     string
		args     args
		expected *InvokeResponse
	}{
		{
			name: "tool_calls",
			args: args{
				res: &openai.ChatCompletionResponse{
					Choices: []openai.ChatCompletionChoice{
						{
							Index: 0,
							Message: openai.ChatCompletionMessage{
								Role:    "user",
								Content: "How is the weather today?",
								ToolCalls: []openai.ToolCall{
									{
										Type:     openai.ToolTypeFunction,
										Function: openai.FunctionCall{Name: "get-weather"},
									},
								},
								ToolCallID: "9TWd1eA2K3rmmtC21oER2a9F0YZif",
							},
							FinishReason: openai.FinishReasonToolCalls,
						},
					},
					Usage: openai.Usage{
						PromptTokens:     50,
						CompletionTokens: 100,
						TotalTokens:      150,
					},
				},
				tools: []openai.Tool{
					{
						Type:     openai.ToolTypeFunction,
						Function: &openai.FunctionDefinition{Name: "get-weather"},
					},
				},
			},
			expected: &InvokeResponse{
				Content: "How is the weather today?",
				ToolCalls: []openai.ToolCall{
					{Type: openai.ToolTypeFunction, Function: openai.FunctionCall{Name: "get-weather"}},
				},
				FinishReason: "tool_calls",
				TokenUsage:   TokenUsage{PromptTokens: 50, CompletionTokens: 100},
				AssistantMessage: openai.ChatCompletionMessage{
					Role:    "user",
					Content: "How is the weather today?",
					ToolCalls: []openai.ToolCall{
						{Type: openai.ToolTypeFunction, Function: openai.FunctionCall{Name: "get-weather"}},
					},
					ToolCallID: "9TWd1eA2K3rmmtC21oER2a9F0YZif",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, _ := ConvertToInvokeResponse(tt.args.res, tt.args.tools)
			assert.Equal(t, tt.expected, actual)
		})
	}
}
