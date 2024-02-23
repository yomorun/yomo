package ai

import (
	"encoding/json"
	"fmt"

	"github.com/yomorun/yomo/serverless"
)

// ReducerTag is the observed tag of the reducer
var ReducerTag uint32 = 0x61

type sseDataValue struct {
	Result    string `json:"result"`
	Arguments string `json:"arguments"`
}

// FunctionCallObject describes the data structure when invoking the sfn function
type FunctionCallObject struct {
	// TransID is the transaction id of the function calling chain, it is used for
	// multi-turn llm request.
	TransID string `json:"tid,omitempty"`
	// ReqID is the request id of the current function calling chain. Because multiple
	// function calling invokes may be occurred in the same request chain.
	ReqID string `json:"reqId"`
	// Arguments is the arguments of the function calling. This should be kept in this
	// context for next llm request in multi-turn request scenario.
	Arguments string `json:"arguments"`
	// Result is the struct result of the function calling.
	Result string `json:"result,omitempty"`
	// RetrievalResult is the string result of the function calling.
	RetrievalResult string `json:"retrievalResult,omitempty"`
	// ctx is the serverless context used in sfn.
	ToolCallID string `json:"toolCallID,omitempty"`
	// FunctionName is the name of the function
	FunctionName string `json:"functionName,omitempty"`
	ctx          *serverless.Context
}

// Bytes serialize the []byte of FunctionCallObject
func (fco *FunctionCallObject) Bytes() ([]byte, error) {
	return json.Marshal(fco)
}

// FromBytes deserialize the FunctionCallObject from the given []byte
func (fco *FunctionCallObject) FromBytes(b []byte) error {
	var obj = &FunctionCallObject{}
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
	return nil
}

// Write writes the result to zipper
func (fco *FunctionCallObject) Write(result string) error {
	// tag, data := fco.CreatePayload(result)
	fco.Result = result
	buf, err := fco.Bytes()
	if err != nil {
		return err
	}
	return (*fco.ctx).Write(ReducerTag, buf)
}

// SetRetrievalResult sets the retrieval result
func (fco *FunctionCallObject) SetRetrievalResult(retrievalResult string) {
	fco.RetrievalResult = retrievalResult
}

// UnmarshalArguments deserialize Arguments to the parameter object
func (fco *FunctionCallObject) UnmarshalArguments(v any) error {
	return json.Unmarshal([]byte(fco.Arguments), v)
}

// NewFunctionCallingInvoke creates a new unctionCallObject from the given context
func ParseFunctionCallContext(ctx serverless.Context) (*FunctionCallObject, error) {
	if ctx == nil {
		return nil, fmt.Errorf("ai: ctx is nil")
	}

	if ctx.Data() == nil {
		return nil, fmt.Errorf("ai: ctx.Data() is nil")
	}

	if len(ctx.Data()) < 6 {
		return nil, fmt.Errorf("ai: ctx.Data() is too short")
	}

	fco := &FunctionCallObject{}
	fco.ctx = &ctx
	fco.FromBytes(ctx.Data())
	return fco, nil
}
