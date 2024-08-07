package serverless

import (
	"encoding/json"
	"errors"

	"github.com/yomorun/yomo/ai"
)

// ReadLLMArguments reads LLM function arguments
func (c *Context) ReadLLMArguments(args any) error {
	fnCall, err := c.LLMFunctionCall()
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
	// check if the c.fnCall is nil, if it is, read the fnCall from c.data and assign to c.fnCall.
	if c.fnCall == nil {
		fnCall, err := c.LLMFunctionCall()
		if err != nil {
			return err
		}
		c.fnCall = fnCall
	}
	if c.fnCall.IsOK && c.fnCall.Result != "" {
		return errors.New("LLM function can only be called once")
	}
	// function call
	c.fnCall.IsOK = true
	c.fnCall.Result = result
	buf, err := c.fnCall.Bytes()
	if err != nil {
		return err
	}
	c.data = buf
	return c.Write(ai.ReducerTag, buf)
}

// LLMFunctionCall reads LLM function call
func (c *Context) LLMFunctionCall() (*ai.FunctionCall, error) {
	fco := &ai.FunctionCall{}
	if err := fco.FromBytes(c.data); err != nil {
		return nil, err
	}

	return fco, nil
}
