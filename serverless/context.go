// Package serverless defines serverless handler context
package serverless

import "io"

// Context sfn handler context
type Context interface {
	// Data incoming data
	Data() []byte
	// Tag incoming tag
	Tag() uint32
	// Write write data to zipper
	Write(tag uint32, data []byte) error
	// HTTP http interface
	HTTP() HTTP
	// Streamed returns whether the data is streamed
	Streamed() bool
	// Stream returns the stream
	Stream() io.Reader
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
