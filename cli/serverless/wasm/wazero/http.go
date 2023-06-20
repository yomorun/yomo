package wazero

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	wasmhttp "github.com/yomorun/yomo/cli/serverless/wasm/http"
	"github.com/yomorun/yomo/serverless"
)

const DefaultHTTPTimeout = 10 * time.Second

type Request struct{}

func ExportHTTPHostFuncs(builder wazero.HostModuleBuilder) {
	builder.
		// get
		NewFunctionBuilder().
		WithGoModuleFunction(
			api.GoModuleFunc(Send),
			[]api.ValueType{
				api.ValueTypeI32, // reqPtr
				api.ValueTypeI32, // reqSize
				api.ValueTypeI32, // respPtr
				api.ValueTypeI32, // respSize
			},
			[]api.ValueType{api.ValueTypeI32}, // ret
		).
		Export(wasmhttp.WasmFuncHTTPSend)
}

// Send sends a HTTP request and returns the response
func Send(ctx context.Context, m api.Module, stack []uint64) {
	// request
	reqPtr := uint32(stack[0])
	reqSize := uint32(stack[1])
	reqBuf, err := readBuffer(ctx, m, reqPtr, reqSize)
	if err != nil {
		log.Printf("[HTTP] Send: get request error: %s\n", err)
		stack[0] = 1
		return
	}
	var req serverless.HTTPRequest
	if err := json.Unmarshal(reqBuf, &req); err != nil {
		log.Printf("[HTTP] Send: unmarshal request error: %s\n", err)
		stack[0] = 2
		return
	}
	// create http client
	// 10 seconds timeout
	timeout := DefaultHTTPTimeout
	if req.Timeout > 0 {
		timeout = time.Duration(req.Timeout * 1e6)
	}

	client := &http.Client{Timeout: timeout}
	// create http request
	reqBody := bytes.NewReader(req.Body)
	request, err := http.NewRequest(req.Method, req.URL, reqBody)
	if err != nil {
		log.Printf("[HTTP] Send: create http request error: %s\n", err)
		stack[0] = 3
		return
	}
	// set headers
	for k, v := range req.Header {
		request.Header.Set(k, v)
	}
	// send http request
	response, err := client.Do(request)
	if err != nil {
		log.Printf("[HTTP] Send: http request error: %s\n", err)
		stack[0] = 4
		return
	}
	defer response.Body.Close()
	// response
	body, err := io.ReadAll(response.Body)
	if err != nil {
		log.Printf("[HTTP] Send: read response body error: %s\n", err)
		stack[0] = 5
		return
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
		log.Printf("[HTTP] Send: marshal response error: %s\n", err)
		stack[0] = 6
		return
	}
	// write response
	respPtr := uint32(stack[2])
	respSize := uint32(stack[3])
	if err := allocateBuffer(ctx, m, respPtr, respSize, respBuf); err != nil {
		log.Printf("[HTTP] Send: write response error: %s\n", err)
		stack[0] = 7
		return
	}
	// return
	stack[0] = 0
}
