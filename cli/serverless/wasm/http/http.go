package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/yomorun/yomo/serverless"
)

const (
	// http
	WasmFuncHTTPSend = "yomo_http_send"
	// DefaultHTTPTimeout is the default timeout for HTTP requests
	DefaultHTTPTimeout = 10 * time.Second
)

// Do sends an HTTP request and returns an HTTP response with buffer
func Do(reqBuf []byte) ([]byte, error) {
	var req serverless.HTTPRequest
	if err := json.Unmarshal(reqBuf, &req); err != nil {
		return nil, fmt.Errorf("unmarshal request error: %s", err)
	}
	// create http client
	timeout := DefaultHTTPTimeout
	if req.Timeout > 0 {
		timeout = time.Duration(req.Timeout * 1e6)
	}
	// create http client
	client := &http.Client{Timeout: timeout}
	// create http request
	reqBody := bytes.NewReader(req.Body)
	request, err := http.NewRequest(req.Method, req.URL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("create http request error: %s", err)
	}
	// set headers
	for k, v := range req.Header {
		request.Header.Set(k, v)
	}
	// send http request
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("do http request error: %s", err)
	}
	defer response.Body.Close()
	// response
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body error: %s", err)
	}
	resp := serverless.HTTPResponse{
		Status:     response.Status,
		StatusCode: response.StatusCode,
		Header:     make(map[string]string),
		Body:       body,
	}
	// response headers
	for k, v := range response.Header {
		if len(v) > 0 {
			resp.Header[k] = v[0]
		}
	}
	// marshal response
	respBuf, err := json.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("marshal response error: %s", err)
	}

	return respBuf, nil
}
