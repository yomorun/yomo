package ai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/go-openai"
)

func TestDecodeChatCompletionRequest(t *testing.T) {
	type args struct {
		data string
	}
	tests := []struct {
		name             string
		args             args
		wantReq          openai.ChatCompletionRequest
		wantAgentContext any
		wantErrString    string
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
			wantAgentContext: nil,
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
			wantAgentContext: nil,
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
			wantAgentContext: nil,
		},
		{
			name: "response_format=nil",
			args: args{
				data: `{"model":"gpt-4o"}`,
			},
			wantReq: openai.ChatCompletionRequest{
				Model: "gpt-4o",
			},
			wantAgentContext: nil,
		},
		{
			name: "not a json",
			args: args{
				data: `model=gpt-4o,response_format=text`,
			},
			wantErrString:    "invalid character 'm' looking for beginning of value",
			wantAgentContext: nil,
		},
		{
			name: "agent_context",
			args: args{
				data: `{"model":"gpt-4o-2024-08-06","response_format":{"type":"json_object"},"agent_context":{"user_id":"123456"}}`,
			},
			wantReq: openai.ChatCompletionRequest{
				Model: "gpt-4o-2024-08-06",
				ResponseFormat: &openai.ChatCompletionResponseFormat{
					Type: openai.ChatCompletionResponseFormatTypeJSONObject,
				},
			},
			wantAgentContext: map[string]any{
				"user_id": "123456",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotReq, gotAgentContext, err := DecodeChatCompletionRequest([]byte(tt.args.data))
			if err != nil {
				assert.EqualError(t, err, tt.wantErrString)
			}
			assert.Equal(t, tt.wantReq, gotReq)
			assert.Equal(t, tt.wantReq.ResponseFormat, gotReq.ResponseFormat)
			assert.Equal(t, tt.wantAgentContext, gotAgentContext)
		})
	}
}
