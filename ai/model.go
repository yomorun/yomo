// Package ai contains the model for LLM Function Calling features
package ai

// ErrorResponse is the response for error
type ErrorResponse struct {
	Error string `json:"error"`
}

// OverviewResponse is the response for overview
type OverviewResponse struct {
	Functions map[uint32]*FunctionDefinition // key is the tag of yomo
}

// InvokeRequest is the request from user to BasicAPIServer
type InvokeRequest struct {
	ReqID            string `json:"req_id"`             // ReqID is the request id of the request
	Prompt           string `json:"prompt"`             // Prompt is user input text for chat completion
	IncludeCallStack bool   `json:"include_call_stack"` // IncludeCallStack is the flag to include call stack in response
}

// InvokeResponse is the response for chat completions
type InvokeResponse struct {
	// Functions is the functions from llm api response, key is the tag of yomo
	// Functions map[uint32][]*FunctionDefinition
	// Content is the content from llm api response
	Content string
	// ToolCalls is the toolCalls from llm api response
	ToolCalls map[uint32][]*ToolCall
	// ToolMessages is the tool messages from llm api response
	ToolMessages []ToolMessage
	// FinishReason is the finish reason from llm api response
	FinishReason string
	// TokenUsage is the token usage from llm api response
	TokenUsage TokenUsage
	// AssistantMessage is the assistant message from llm api response, only present when finish reason is "tool_calls"
	AssistantMessage interface{}
}

// TokenUsage is the token usage in Response
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// ToolCall is the tool call in Request and Response
type ToolCall struct {
	ID       string              `json:"id,omitempty"` // ID present in Response only
	Type     string              `json:"type"`
	Function *FunctionDefinition `json:"function"`
}

// Equal compares two ToolCall function
// return true if type and function name are same
func (t ToolCall) Equal(tool ToolCall) bool {
	if t.Type == tool.Type && t.Function.Name == tool.Function.Name {
		return true
	}
	return false
}

// FunctionDefinition is the function definition
type FunctionDefinition struct {
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	Parameters  *FunctionParameters `json:"parameters,omitempty"` // chatCompletionFunctionParameters
	Arguments   string              `json:"arguments,omitempty"`  // not used in request
}

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
	ToolCallId string `json:"tool_call_id"`
}

// ChainMessage is the message for chaining llm request with preceeding `tool_calls` response
type ChainMessage struct {
	// PrecedingAssistantMessage is the preceding assistant message in llm response
	PreceedingAssistantMessage interface{}
	// ToolMessages is the tool messages aggragated from reducer-sfn by AI service
	ToolMessages []ToolMessage
}
