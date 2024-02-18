package ai

// OverviewResponse is the response for overview
type OverviewResponse struct {
	Functions map[uint32]*FunctionDefinition // key is the tag of yomo
}

// ChatCompletionsRequest is the request for chat completions
type ChatCompletionsRequest struct {
	ReqID string `json:"req_id"` // req_id is the request id of the request
	// AppID  string `json:"app_id"`  // app_id is the app id of allegro application, it's empty in the yomo
	// PeerID string `json:"peer_id"` // peer_id is the tenant id of application
	Prompt string `json:"prompt"` // prompt is user input text for chat completion
}

// ChatCompletionsResponse is the response for chat completions
type ChatCompletionsResponse struct {
	Functions map[uint32][]*FunctionDefinition // key is the tag of yomo
	Content   string
}

// ToolCall is the tool call in Request and Response
type ToolCall struct {
	ID       string              `json:"id,omitempty"` // ID present in Response only
	Type     string              `json:"type"`
	Function *FunctionDefinition `json:"function"`
}

// Equal compares two ToolCall function
// return true if type and function name are same
func (t *ToolCall) Equal(tool *ToolCall) bool {
	if t.Type == tool.Type && t.Function.Name == tool.Function.Name {
		return true
	}
	return false
}

// FunctionDefinition is the function definition
type FunctionDefinition struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Parameters  *FunctionParameters `json:"parameters,omitempty"` // chatCompletionFunctionParameters
	Arguments   string              `json:"arguments,omitempty"`
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
