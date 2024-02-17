package ai

import (
	"fmt"

	"github.com/yomorun/yomo/serverless"
)

type ChatCompletionsRequest struct {
	ReqID string `json:"req_id"` // req_id is the request id of the request
	// AppID  string `json:"app_id"`  // app_id is the app id of allegro application, it's empty in the yomo
	// PeerID string `json:"peer_id"` // peer_id is the tenant id of application
	Prompt string `json:"prompt"` // prompt is user input text for chat completion
}

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

// SfnInvokeParameters describes the data structure when invoking the sfn function
type SfnInvokeParameters struct {
	ReqID     string // ReqID is the request id of the restful request
	Arguments string // Arguments is the arguments of the function calling
}

// Bytes returns the byte slice of SfnInvokeParameters
func (sip *SfnInvokeParameters) Bytes() []byte {
	buf1 := []byte(sip.ReqID)
	buf2 := []byte(sip.Arguments)
	return append(buf1, buf2...)
}

// FromBytes fills the SfnInvokeParameters from the given byte slice
func (sip *SfnInvokeParameters) FromBytes(b []byte) {
	sip.ReqID = string(b[:6])
	sip.Arguments = string(b[6:])
}

// CreatePayload creates the payload from the given result
func (sip *SfnInvokeParameters) CreatePayload(result string) (uint32, []byte) {
	sip.Arguments = result
	return 0x61, sip.Bytes()
}

// NewFunctionCallingInvoke creates a new SfnInvokeParameters from the given context
func NewFunctionCallingInvoke(ctx serverless.Context) (*SfnInvokeParameters, error) {
	if ctx == nil {
		return nil, fmt.Errorf("ai: ctx is nil")
	}

	if ctx.Data() == nil {
		return nil, fmt.Errorf("ai: ctx.Data() is nil")
	}

	if len(ctx.Data()) < 6 {
		return nil, fmt.Errorf("ai: ctx.Data() is too short")
	}

	sip := &SfnInvokeParameters{}
	sip.FromBytes(ctx.Data())
	return sip, nil
}
