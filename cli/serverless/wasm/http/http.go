package http

import "time"

const (
	// http
	WasmFuncHTTPSend = "yomo_http_send"
	// DefaultHTTPTimeout is the default timeout for HTTP requests
	DefaultHTTPTimeout = 10 * time.Second
)
