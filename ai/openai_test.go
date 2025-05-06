package ai

import (
	"encoding/json"
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

func TestDecodeChatCompletionRequest(t *testing.T) {
	type args struct {
		data string
	}
	tests := []struct {
		name          string
		args          args
		wantReq       openai.ChatCompletionRequest
		wantErrString string
	}{
		{
			name: "response_format=json_schema",
			args: args{
				data: `{"model":"gpt-4o","response_format":{"type":"json_schema","json_schema":{"name":"math_reasoning","schema":{"type":"object","properties":{"steps":{"type":"array","items":{"type":"object","properties":{"explanation":{"type":"string"},"output":{"type":"string"}},"required":["explanation","output"],"additionalProperties":false}},"final_answer":{"type":"string"}},"required":["steps","final_answer"],"additionalProperties":false},"strict":true}}}`,
			},
			wantReq: openai.ChatCompletionRequest{
				Model: "gpt-4o",
				ResponseFormat: &openai.ChatCompletionResponseFormat{
					Type: openai.ChatCompletionResponseFormatTypeJSONSchema,
					JSONSchema: &openai.ChatCompletionResponseFormatJSONSchema{
						Name:   "math_reasoning",
						Schema: json.RawMessage(`{"type":"object","properties":{"steps":{"type":"array","items":{"type":"object","properties":{"explanation":{"type":"string"},"output":{"type":"string"}},"required":["explanation","output"],"additionalProperties":false}},"final_answer":{"type":"string"}},"required":["steps","final_answer"],"additionalProperties":false}`),
						Strict: true,
					},
				},
			},
		},
		{
			name: "response_format=json_object",
			args: args{
				data: `{"model":"gpt-4o-2024-08-06","response_format":{"type":"json_object"}}`,
			},
			wantReq: openai.ChatCompletionRequest{
				Model: "gpt-4o-2024-08-06",
				ResponseFormat: &openai.ChatCompletionResponseFormat{
					Type: openai.ChatCompletionResponseFormatTypeJSONObject,
				},
			},
		},
		{
			name: "response_format=text",
			args: args{
				data: `{"model":"gpt-4o","response_format":{"type":"text"}}`,
			},
			wantReq: openai.ChatCompletionRequest{
				Model: "gpt-4o",
				ResponseFormat: &openai.ChatCompletionResponseFormat{
					Type: openai.ChatCompletionResponseFormatTypeText,
				},
			},
		},
		{
			name: "response_format=nil",
			args: args{
				data: `{"model":"gpt-4o"}`,
			},
			wantReq: openai.ChatCompletionRequest{
				Model: "gpt-4o",
			},
		},
		{
			name: "not a json",
			args: args{
				data: `model=gpt-4o,response_format=text`,
			},
			wantErrString: "invalid character 'm' looking for beginning of value",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotReq, err := DecodeChatCompletionRequest([]byte(tt.args.data))
			if err != nil {
				assert.EqualError(t, err, tt.wantErrString)
			}
			assert.Equal(t, tt.wantReq, gotReq)
		})
	}
}
