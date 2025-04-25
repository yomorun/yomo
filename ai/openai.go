package ai

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/sashabaranov/go-openai"
	"github.com/yomorun/yomo/core/ylog"
)

// ConvertToInvokeResponse converts openai.ChatCompletionResponse struct to InvokeResponse struct.
func ConvertToInvokeResponse(res *openai.ChatCompletionResponse, tools []openai.Tool) (*InvokeResponse, error) {
	choice := res.Choices[0]
	ylog.Debug(">>finish_reason", "reason", choice.FinishReason)
	responseMessage := res.Choices[0].Message
	calls := responseMessage.ToolCalls
	ylog.Debug("--response calls", "calls", len(calls))
	content := responseMessage.Content

	result := &InvokeResponse{}
	// finish reason
	result.FinishReason = string(choice.FinishReason)
	result.Content = content

	// record usage
	result.TokenUsage = TokenUsage{
		PromptTokens:     res.Usage.PromptTokens,
		CompletionTokens: res.Usage.CompletionTokens,
	}
	ylog.Debug("++ llm result", "token_usage", fmt.Sprintf("%v", result.TokenUsage), "finish_reason", result.FinishReason)

	// if llm said no function call, we should return the result
	if result.FinishReason == string(openai.FinishReasonStop) {
		return result, nil
	}

	if result.FinishReason == "tool_calls" {
		// assistant message
		result.AssistantMessage = responseMessage
	}

	if len(calls) == 0 {
		return result, errors.New("finish_reason is tool_calls, but no tool calls found")
	}

	for _, call := range calls {
		for _, tc := range tools {
			ylog.Debug(">> compare tool call", "tc", tc.Function.Name, "call", call.Function.Name)
			if tc.Function.Name == call.Function.Name && tc.Type == call.Type {
				if result.ToolCalls == nil {
					result.ToolCalls = make([]openai.ToolCall, 0)
				}
				result.ToolCalls = append(result.ToolCalls, call)
			}
		}
	}

	return result, nil
}

// DecodeChatCompletionRequest decodes openai.ChatCompletionRequest from JSON data.
// If a response_format is present, we cannot directly unmarshal it as a ChatCompletionRequest
// because the schema field is a json.Unmarshaler.
// Therefore, we need to use a temporary tmpRequest to unmarshal it.
func DecodeChatCompletionRequest(data []byte) (req openai.ChatCompletionRequest, err error) {
	type tmpRequest struct {
		openai.ChatCompletionRequest
		ResponseFormat *struct {
			*openai.ChatCompletionResponseFormat
			JSONSchema *struct {
				*openai.ChatCompletionResponseFormatJSONSchema
				Schema json.RawMessage `json:"schema"` // json.RawMessage implements json.Unmarshaler
			} `json:"json_schema"`
		} `json:"response_format"`
	}

	var tmp tmpRequest
	if err := json.Unmarshal(data, &tmp); err != nil {
		return openai.ChatCompletionRequest{}, err
	}

	req = tmp.ChatCompletionRequest

	if format := tmp.ResponseFormat; format != nil {
		req.ResponseFormat = format.ChatCompletionResponseFormat
		if jsonSchema := format.JSONSchema; jsonSchema != nil {
			req.ResponseFormat.JSONSchema = jsonSchema.ChatCompletionResponseFormatJSONSchema
			if schema := format.JSONSchema.Schema; schema != nil {
				format.JSONSchema.ChatCompletionResponseFormatJSONSchema.Schema = schema
			}
		}
	}

	return req, nil
}
