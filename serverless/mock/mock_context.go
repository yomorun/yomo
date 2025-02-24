// Package mock provides a mock context for stream function.
package mock

import (
	"encoding/json"
	"errors"
	"sync"

	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/pkg/id"
	"github.com/yomorun/yomo/serverless"
)

var _ serverless.Context = (*MockContext)(nil)

// WriteRecord composes the data, tag and target.
type WriteRecord struct {
	Data      []byte
	Tag       uint32
	Target    string
	LLMResult string
}

// MockContext mock context.
type MockContext struct {
	data   []byte
	tag    uint32
	fnCall *ai.FunctionCall

	mu      sync.Mutex
	wrSlice []WriteRecord
}

// NewMockContext returns the mock context.
// the data is that returned by ctx.Data(), the tag is that returned by ctx.Tag().
func NewMockContext(data []byte, tag uint32) *MockContext {
	return &MockContext{
		data: data,
		tag:  tag,
	}
}

// NewArgumentsContext creates a Context with the provided arguments and tag.
// This function is used for testing the LLM function.
func NewArgumentsContext(arguments string, tag uint32) *MockContext {
	fnCall := &ai.FunctionCall{
		Arguments:  arguments,
		ReqID:      id.New(16),
		ToolCallID: "chatcmpl-" + id.New(29),
	}
	data, _ := fnCall.Bytes()

	return &MockContext{
		data:   data,
		tag:    tag,
		fnCall: fnCall,
	}
}

// Data incoming data.
func (c *MockContext) Data() []byte {
	return c.data
}

// Tag incoming tag.
func (c *MockContext) Tag() uint32 {
	return c.tag
}

// Metadata returns the metadata by the given key.
func (c *MockContext) Metadata(_ string) (string, bool) {
	panic("not implemented")
}

// HTTP returns the HTTP interface.H
func (c *MockContext) HTTP() serverless.HTTP {
	panic("not implemented, to use `net/http` package")
}

// Write writes the data with the given tag.
func (c *MockContext) Write(tag uint32, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.wrSlice = append(c.wrSlice, WriteRecord{
		Data: data,
		Tag:  tag,
	})

	return nil
}

// WriteWithTarget writes the data with the given tag and target.
func (c *MockContext) WriteWithTarget(tag uint32, data []byte, target string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.wrSlice = append(c.wrSlice, WriteRecord{
		Data:   data,
		Tag:    tag,
		Target: target,
	})

	return nil
}

// ReadLLMArguments reads LLM function arguments.
func (c *MockContext) ReadLLMArguments(args any) error {
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

// WriteLLMResult writes LLM function result.
func (c *MockContext) WriteLLMResult(result string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

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

	c.wrSlice = append(c.wrSlice, WriteRecord{
		Data:      buf,
		Tag:       ai.ReducerTag,
		LLMResult: result,
	})

	return nil
}

// LLMFunctionCall reads LLM function call
func (c *MockContext) LLMFunctionCall() (*ai.FunctionCall, error) {
	fco := &ai.FunctionCall{}
	if err := fco.FromBytes(c.data); err != nil {
		return nil, err
	}

	return fco, nil
}

// RecordsWritten returns the data records be written with `ctx.Write`.
func (c *MockContext) RecordsWritten() []WriteRecord {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.wrSlice
}
