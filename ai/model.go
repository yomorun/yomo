// Package ai contains the model for LLM Function Calling features
package ai

import "github.com/sashabaranov/go-openai"

// ErrorResponse is the response for error
type ErrorResponse struct {
	Error string `json:"error"`
}

// OverviewResponse is the response for overview
type OverviewResponse struct {
	Functions map[uint32]*openai.FunctionDefinition // key is the tag of yomo
}

// InvokeRequest is the request from user to BasicAPIServer
type InvokeRequest struct {
	Prompt           string `json:"prompt"`             // Prompt is user input text for chat completion
	IncludeCallStack bool   `json:"include_call_stack"` // IncludeCallStack is the flag to include call stack in response
}

// InvokeResponse is the response for chat completions
type InvokeResponse struct {
	// Content is the content from llm api response
	Content string `json:"content,omitempty"`
	// ToolCalls is the toolCalls from llm api response
	ToolCalls map[uint32][]*openai.ToolCall `json:"tool_calls,omitempty"`
	// ToolMessages is the tool messages from llm api response
	ToolMessages []openai.ChatCompletionMessage `json:"tool_messages,omitempty"`
	// FinishReason is the finish reason from llm api response
	FinishReason string `json:"finish_reason,omitempty"`
	// TokenUsage is the token usage from llm api response
	TokenUsage TokenUsage `json:"token_usage,omitempty"`
	// AssistantMessage is the assistant message from llm api response, only present when finish reason is "tool_calls"
	AssistantMessage interface{} `json:"assistant_message,omitempty"`
}

// TokenUsage is the token usage in Response
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// FunctionDefinition is the function definition
type FunctionDefinition = openai.FunctionDefinition

// FunctionParameters defines the parameters the functions accepts.
// from API doc: "The parameters the functions accepts, described as a JSON Schema object. See the [guide](/docs/guides/gpt/function-calling) for examples, and the [JSON Schema reference](https://json-schema.org/understanding-json-schema/) for documentation about the format."
type FunctionParameters struct {
	Type       string                        `json:"type"`
	Properties map[string]*ParameterProperty `json:"properties"`
	Required   []string                      `json:"required"`
}

// ParameterProperty defines the property of the parameter
type ParameterProperty struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Enum        []string `json:"enum,omitempty"`
}

// ToolMessage used for OpenAI tool message
type ToolMessage struct {
	Role       string `json:"role"`
	Content    string `json:"content"`
	ToolCallID string `json:"tool_call_id"`
}

// ChainMessage is the message for chaining llm request with preceeding `tool_calls` response
type ChainMessage struct {
	// PrecedingAssistantMessage is the preceding assistant message in llm response
	PreceedingAssistantMessage interface{}
	// ToolMessages is the tool messages aggragated from reducer-sfn by AI service
	ToolMessages []ToolMessage
}

// FunctionDefinitionKey is the yomo metadata key for function definition
const FunctionDefinitionKey = "function-definition"
