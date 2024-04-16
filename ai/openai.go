package ai

import (
	"errors"
	"fmt"
	"strings"

	"github.com/sashabaranov/go-openai"
	"github.com/yomorun/yomo/core/ylog"
)

// ChatCompletionRequest represents a request structure for chat completion API
type ChatCompletionRequest openai.ChatCompletionRequest

// ChatCompletionMessage represents a message structure for chat completion API
type ChatCompletionMessage openai.ChatCompletionMessage

// ChatCompletionResponseFormat represents the response format for chat completion API
type ChatCompletionResponseFormat struct {
	Type string `json:"type,omitempty"`
}

// ChatCompletionResponseFormat represents the response format for chat completion API
type ChatCompletionResponse openai.ChatCompletionResponse

// ChatCompletionChoice represents the choice in chat completion API
type ChatCompletionChoice struct {
	Index        int                   `json:"index"`
	Message      ChatCompletionMessage `json:"message"`
	FinishReason string                `json:"finish_reason"`
	// LogProbs     *LogProbs    `json:"logprobs,omitempty"`
}

// Usage Represents the total token usage per request to OpenAI
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

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
	// gemini provider will return the finish_reason is "STOP", otherwise "stop"
	if strings.ToLower(result.FinishReason) == "stop" {
		return result, nil
	}

	if result.FinishReason == "tool_calls" || result.FinishReason == "gemini_tool_calls" {
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
			// WARN: gemini process tool calls, currently function name not equal to tool call name, eg. "get-weather" != "get_weather"
			if (tc.Function.Name == call.Function.Name && tc.Type == call.Type) || result.FinishReason == "gemini_tool_calls" {
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
