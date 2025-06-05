// Package serverless defines serverless handler context
package serverless

import "github.com/yomorun/yomo/ai"

// Context sfn handler context
type Context interface {
	// Data incoming data
	Data() []byte
	// Tag incoming tag
	Tag() uint32
	// Metadata incoming metadata
	Metadata(string) (string, bool)
	// Write writes data
	Write(tag uint32, data []byte) error
	// WriteWithTarget writes data to sfn instance with specified target
	WriteWithTarget(tag uint32, data []byte, target string) error
	// ReadLLMArguments reads LLM function arguments
	ReadLLMArguments(args any) error
	// WriteLLMResult writes LLM function result
	WriteLLMResult(result string) error
	// LLMFunctionCall reads LLM function call
	LLMFunctionCall() (*ai.FunctionCall, error)
}

// CronContext sfn corn handler context
type CronContext interface {
	// Write writes data
	Write(tag uint32, data []byte) error
	// WriteWithTarget writes data to sfn instance with specified target
	WriteWithTarget(tag uint32, data []byte, target string) error
}
