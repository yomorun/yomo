package serverless

import (
	"encoding/json"
	"errors"

	"github.com/yomorun/yomo/ai"
)

// ReadLLMArguments reads LLM function arguments
func (c *Context) ReadLLMArguments(args any) error {
	fnCall := &ai.FunctionCall{}
	err := fnCall.FromBytes(c.data)
	if err != nil {
		return err
	}
	// if success, assign the object to the given object
	c.fnCall = fnCall
	if len(fnCall.Arguments) == 0 && args != nil {
		return errors.New("function arguments is empty, can't read to the given object")
	}
	return json.Unmarshal([]byte(fnCall.Arguments), args)
}

// WriteLLMResult writes LLM function result
func (c *Context) WriteLLMResult(result string) error {
	if c.fnCall == nil {
		return errors.New("no function call, can't write result")
	}
	// function call
	c.fnCall.IsOK = true
	c.fnCall.Result = result
	buf, err := c.fnCall.Bytes()
	if err != nil {
		return err
	}
	return c.Write(ai.ReducerTag, buf)
}

// ReadLLMFunctionCall reads LLM function call
func (c *Context) ReadLLMFunctionCall(fnCall any) error {
	if c.data == nil {
		return errors.New("ctx.Data() is nil")
	}
	fco, ok := fnCall.(*ai.FunctionCall)
	if !ok {
		return errors.New("given object is not *ai.FunctionCall")
	}
	return fco.FromBytes(c.data)
}
