package ai

import (
	"encoding/json"
	"fmt"

	"github.com/yomorun/yomo/serverless"
)

// ReducerTag is the observed tag of the reducer
var ReducerTag uint32 = 0x61

// FunctionCall describes the data structure when invoking the sfn function
type FunctionCall struct {
	// TransID is the transaction id of the function calling chain, it is used for
	// multi-turn llm request.
	TransID string `json:"tid,omitempty"`
	// ReqID is the request id of the current function calling chain. Because multiple
	// function calling invokes may be occurred in the same request chain.
	ReqID string `json:"req_id"`
	// Arguments is the arguments of the function calling. This should be kept in this
	// context for next llm request in multi-turn request scenario.
	Arguments string `json:"arguments"`
	// Result is the struct result of the function calling.
	Result string `json:"result,omitempty"`
	// RetrievalResult is the string result of the function calling.
	RetrievalResult string `json:"retrieval_result,omitempty"`
	// ctx is the serverless context used in sfn.
	ToolCallID string `json:"tool_call_id,omitempty"`
	// FunctionName is the name of the function
	FunctionName string `json:"function_name,omitempty"`
	// IsOK is the flag to indicate the function calling is ok or not
	IsOK bool `json:"is_ok"`
	// Error is the error message
	Error string `json:"error,omitempty"`
	ctx   *serverless.Context
}

// Bytes serialize the []byte of FunctionCallObject
func (fco *FunctionCall) Bytes() ([]byte, error) {
	return json.Marshal(fco)
}

// FromBytes deserialize the FunctionCallObject from the given []byte
func (fco *FunctionCall) FromBytes(b []byte) error {
	obj := &FunctionCall{}
	err := json.Unmarshal(b, &obj)
	if err != nil {
		return err
	}
	fco.TransID = obj.TransID
	fco.ReqID = obj.ReqID
	fco.Arguments = obj.Arguments
	fco.FunctionName = obj.FunctionName
	fco.ToolCallID = obj.ToolCallID
	fco.Result = obj.Result
	fco.RetrievalResult = obj.RetrievalResult
	fco.IsOK = obj.IsOK
	return nil
}

// Write writes the result to zipper
func (fco *FunctionCall) Write(result string) error {
	// tag, data := fco.CreatePayload(result)
	fco.Result = result
	fco.IsOK = true
	buf, err := fco.Bytes()
	if err != nil {
		return err
	}
	return (*fco.ctx).Write(ReducerTag, buf)
}

// WriteErrors writes the error to reducer
func (fco *FunctionCall) WriteErrors(err error) error {
	fco.IsOK = false
	fco.Error = err.Error()
	return fco.Write("")
}

// SetRetrievalResult sets the retrieval result
func (fco *FunctionCall) SetRetrievalResult(retrievalResult string) {
	fco.IsOK = true
	fco.RetrievalResult = retrievalResult
}

// UnmarshalArguments deserialize Arguments to the parameter object
func (fco *FunctionCall) UnmarshalArguments(v any) error {
	return json.Unmarshal([]byte(fco.Arguments), v)
}

// JSONString returns the JSON string of FunctionCallObject
func (fco *FunctionCall) JSONString() string {
	b, _ := json.Marshal(fco)
	return string(b)
}

// ParseFunctionCallContext creates a new unctionCallObject from the given context
func ParseFunctionCallContext(ctx serverless.Context) (*FunctionCall, error) {
	if ctx == nil {
		return nil, fmt.Errorf("ai: ctx is nil")
	}

	if ctx.Data() == nil {
		return nil, fmt.Errorf("ai: ctx.Data() is nil")
	}

	if len(ctx.Data()) < 6 {
		return nil, fmt.Errorf("ai: ctx.Data() is too short")
	}

	fco := &FunctionCall{
		IsOK: true,
	}
	fco.ctx = &ctx
	fco.FromBytes(ctx.Data())
	return fco, nil
}
