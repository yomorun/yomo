package ai

import (
	"errors"
	"fmt"

	"github.com/sashabaranov/go-openai"
	"github.com/yomorun/yomo/core/ylog"
)

// ConvertToInvokeResponse converts openai.ChatCompletionResponse struct to InvokeResponse struct.
func ConvertToInvokeResponse(res *openai.ChatCompletionResponse, tcs map[uint32]openai.Tool) (*InvokeResponse, error) {
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
	if result.FinishReason == string(openai.FinishReasonStop) && len(calls) == 0 {
		return result, nil
	}

	if result.FinishReason == "tool_calls" {
		// assistant message
		result.AssistantMessage = responseMessage
	}

	if len(calls) == 0 {
		return result, errors.New("finish_reason is tool_calls, but no tool calls found")
	}

	// functions may be more than one
	for _, call := range calls {
		for tag, tc := range tcs {
			ylog.Debug(">> compare tool call", "tag", tag, "tc", tc.Function.Name, "call", call.Function.Name)
			if tc.Function.Name == call.Function.Name && tc.Type == call.Type {
				if result.ToolCalls == nil {
					result.ToolCalls = make(map[uint32][]*openai.ToolCall)
				}
				// create a new variable to hold the current call
				currentCall := call
				result.ToolCalls[tag] = append(result.ToolCalls[tag], &currentCall)
			}
		}
	}

	return result, nil
}
