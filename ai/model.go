// Package ai contains the model for LLM Function Calling features
package ai

import "github.com/yomorun/go-openai"

// ErrorResponse is the response for error
type ErrorResponse struct {
	Error string `json:"error"`
}

// OverviewResponse is the response for overview
type OverviewResponse struct {
	Functions []*openai.FunctionDefinition
}

// InvokeRequest is the request from user to BasicAPIServer
type InvokeRequest struct {
	Prompt           string `json:"prompt"`             // Prompt is user input text for chat completion
	AgentContext     any    `json:"agent_context"`      // AgentContext is the context for llm api request
	IncludeCallStack bool   `json:"include_call_stack"` // IncludeCallStack is the flag to include call stack in response
}

// InvokeResponse is the response for chat completions
type InvokeResponse struct {
	// Content is the content from llm api response
	Content string `json:"content,omitempty"`
	// FinishReason is the finish reason from llm api response
	FinishReason string `json:"finish_reason,omitempty"`
	// TokenUsage is the token usage from llm api response
	TokenUsage TokenUsage `json:"token_usage,omitempty"`
	// History is the history messages for llm api reqiest
	History []openai.ChatCompletionMessage `json:"history,omitempty"`
}

// TokenUsage is the token usage
type TokenUsage struct {
	// PromptTokens is the prompt tokens
	PromptTokens int `json:"prompt_tokens"`
	// CompletionTokens is the completion tokens
	CompletionTokens int `json:"completion_tokens"`
}

// FunctionDefinition is the function definition
type FunctionDefinition = openai.FunctionDefinition

// FunctionParameters defines the parameters the functions accepts.
// from API doc: "The parameters the functions accepts, described as a JSON Schema object. See the [guide](/docs/guides/gpt/function-calling) for examples, and the [JSON Schema reference](https://json-schema.org/understanding-json-schema/) for documentation about the format."
type FunctionParameters struct {
	Type       string                        `json:"type"`
	Properties map[string]*ParameterProperty `json:"properties"`
	Required   []string                      `json:"required,omitempty"`
}

// ParameterProperty defines the property of the parameter
type ParameterProperty struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Enum        []any  `json:"enum,omitempty"`
}
