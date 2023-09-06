package guest

import (
	"encoding/json"
	"fmt"
	"log"

	_ "unsafe"

	"github.com/yomorun/yomo/serverless"
)

// HTTP is the interface for HTTP request, but it is not implemented in the server side
func (c *GuestContext) HTTP() serverless.HTTP {
	return &GuestHTTP{}
}

// GuestHTTP is the http client for guest
type GuestHTTP struct{}

// Send send http request and return http response
func (g *GuestHTTP) Send(req *serverless.HTTPRequest) (*serverless.HTTPResponse, error) {
	return g.send(req)
}

// Get send http GET request and return http response
func (g *GuestHTTP) Get(url string) (*serverless.HTTPResponse, error) {
	req := &serverless.HTTPRequest{
		Method: "GET",
		URL:    url,
	}
	return g.send(req)
}

// Post send http POST request and return http response
func (g *GuestHTTP) Post(
	url string,
	contentType string,
	body []byte,
) (*serverless.HTTPResponse, error) {
	req := &serverless.HTTPRequest{
		Method: "POST",
		URL:    url,
		Header: map[string]string{"Content-Type": contentType},
		Body:   body,
	}
	return g.send(req)
}

func (g *GuestHTTP) send(req *serverless.HTTPRequest) (*serverless.HTTPResponse, error) {
	// request
	reqBuf, err := json.Marshal(req)
	if err != nil {
		log.Printf("[GuestHTTP] Send: marshal request error: %s\n", err)
		return nil, err
	}
	reqPtr, reqSize := bufferToPtrSize(reqBuf)
	// do http request
	var respPtr *uint32
	var respSize uint32
	if errCode := httpSend(reqPtr, reqSize, &respPtr, &respSize); errCode != 0 {
		err := fmt.Errorf("http request error: %d", errCode)
		log.Printf("[GuestHTTP] Send: %s\n", err)
		return nil, err
	}
	// response
	respBuf := readBufferFromMemory(respPtr, respSize)
	if len(respBuf) == 0 {
		err := fmt.Errorf("http response is empty")
		log.Printf("[GuestHTTP] Send: %s\n", err)
		return nil, err
	}
	var resp serverless.HTTPResponse
	if err := json.Unmarshal(respBuf, &resp); err != nil {
		log.Printf("[GuestHTTP] Send: unmarshal response error: %s\n", err)
		return nil, err
	}
	return &resp, nil
}

//export yomo_http_send
//go:linkname httpSend
func httpSend(reqPtr uintptr, reqSize uint32, respPtr **uint32, respSize *uint32) uint32
