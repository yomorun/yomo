package ai

import (
	"encoding/json"
	"errors"
)

// ReducerTag is the observed tag of the reducer
var ReducerTag uint32 = 0xE001

// FunctionCall describes the data structure when invoking the sfn function
type FunctionCall struct {
	// TransID is the transaction id of the function calling chain, it is used for
	// multi-turn llm request.
	TransID string `json:"tid,omitempty"`
	// ReqID is the request id of the current function calling chain. Because multiple
	// function calling invokes may be occurred in the same request chain.
	ReqID string `json:"req_id"`
	// Result is the struct result of the function calling.
	Result string `json:"result,omitempty"`
	// Arguments is the arguments of the function calling. This should be kept in this
	// context for next llm request in multi-turn request scenario.
	Arguments string `json:"arguments"`
	// ctx is the serverless context used in sfn.
	ToolCallID string `json:"tool_call_id,omitempty"`
	// FunctionName is the name of the function
	FunctionName string `json:"function_name,omitempty"`
	// IsOK is the flag to indicate the function calling is ok or not
	IsOK bool `json:"is_ok"`
}

// Bytes serialize the []byte of FunctionCallObject
func (fco *FunctionCall) Bytes() ([]byte, error) {
	return json.Marshal(fco)
}

// FromBytes deserialize the FunctionCallObject from the given []byte
func (fco *FunctionCall) FromBytes(b []byte) error {
	if b == nil {
		return errors.New("llm-sfn: cannot read data from context")
	}
	err := json.Unmarshal(b, fco)
	if err != nil || fco.ReqID == "" {
		return errors.New("llm-sfn: cannot read function call object from context data")
	}
	return nil
}
