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
	// HTTP http interface
	HTTP() HTTP
	// WriteWithTarget writes data to sfn instance with specified target
	WriteWithTarget(tag uint32, data []byte, target string) error
	// ReadLLMArguments reads LLM function arguments
	ReadLLMArguments(args any) error
	// WriteLLMResult writes LLM function result
	WriteLLMResult(result string) error
	// ReadLLMFunctionCall reads LLM function call
	ReadLLMFunctionCall() (*ai.FunctionCall, error)
}

// CronContext sfn corn handler context
type CronContext interface {
	// Write writes data
	Write(tag uint32, data []byte) error
	// HTTP http interface
	HTTP() HTTP
	// WriteWithTarget writes data to sfn instance with specified target
	WriteWithTarget(tag uint32, data []byte, target string) error
}

// HTTP http interface
type HTTP interface {
	Send(req *HTTPRequest) (*HTTPResponse, error)
	Get(url string) (*HTTPResponse, error)
	Post(url string, contentType string, body []byte) (*HTTPResponse, error)
}

// HTTPRequest http request
type HTTPRequest struct {
	Method  string            // GET, POST, PUT, DELETE, ...
	URL     string            // https://example.org
	Header  map[string]string // {"Content-Type": "application/json"}
	Timeout int64             // timeout in milliseconds
	Body    []byte            // request body
}

// HTTPResponse http response
type HTTPResponse struct {
	Status     string            // "200 OK"
	StatusCode int               // 200, 404, ...
	Header     map[string]string // {"Content-Type": "application/json"}
	Body       []byte            // response body
}
