package ai

import (
	"encoding/json"
	"fmt"

	"github.com/yomorun/yomo/core/ylog"
	"github.com/yomorun/yomo/serverless"
)

// ReducerTag is the observed tag of the reducer
var ReducerTag uint32 = 0x61

type sseDataValue struct {
	Result    string `json:"result"`
	Arguments string `json:"arguments"`
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

// CreatePayload creates the payload for ctx.Write()
func (sip *SfnInvokeParameters) CreatePayload(result string) (uint32, []byte) {
	val := &sseDataValue{
		Result:    result,
		Arguments: sip.Arguments,
	}

	// serialize val to json string
	jsonStr, err := json.Marshal(val)
	if err != nil {
		return ReducerTag, []byte(fmt.Sprintf(">>>>>json.Marshal error: %v", err))
	}
	ylog.Debug("CreatePayload", "jsonStr", string(jsonStr))

	sip.Arguments = string(jsonStr)
	return ReducerTag, sip.Bytes()
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
