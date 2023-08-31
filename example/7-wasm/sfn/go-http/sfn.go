package main

import (
	"fmt"
	"log"

	"github.com/yomorun/yomo/serverless"
	"github.com/yomorun/yomo/serverless/guest"
)

func main() {
	guest.DataTags = DataTags
	guest.Handler = Handler
	guest.Init = Init
}

func Init() error {
	fmt.Println("[SFN] init")
	return nil
}

func Handler(ctx serverless.Context) {
	log.Println("[SFN] ------------------------- BEGIN -------------------------")
	// load input data
	tag := ctx.Tag()
	input := ctx.Data()
	fmt.Printf("[SFN] received %d bytes with tag[%#x]\n", len(input), tag)

	// process app data
	// output := strings.ToUpper(string(input))

	// Get https://example.org
	resp, err := ctx.HTTP().Get("https://example.org")
	if err != nil {
		fmt.Printf("[SFN] execute `HTTP().Get()` request error: %s\n", err)
		return
	}
	log.Printf(
		"[SFN] execute `HTTP().Get()` response result: content-type=%s, body=%s\n",
		resp.Header["Content-Type"],
		resp.Body,
	)
	// Post form post
	host := "https://httpbin.org"
	resp, err = ctx.HTTP().Post(
		host+"/post",
		"application/x-www-form-urlencoded",
		[]byte(
			"custname=customer&custtel=332323&custemail=foo%40bar.com&size=medium&topping=bacon&topping=cheese&delivery=11%3A45&comments=abcdefg",
		),
	)
	if err != nil {
		fmt.Printf("[SFN] execute `HTTP().Post()` request error: %s\n", err)
		return
	}
	log.Printf(
		"[SFN] execute `HTTP().Post()` response result: content-type=%s, body=%s\n",
		resp.Header["Content-Type"],
		resp.Body,
	)
	// httpbin.org test
	req := &serverless.HTTPRequest{
		Method:  "GET",
		URL:     host + "/get",
		Timeout: 3000, // 3s
	}
	// send http GET request
	resp, err = ctx.HTTP().Send(req)
	if err != nil {
		fmt.Printf("[SFN] execute http GET request error: %s\n", err)
		return
	}
	display(req, resp)
	// send http GET request with header
	req.Method = "GET"
	req.URL = host + "/json"
	req.Body = []byte("hello world")
	req.Header = map[string]string{
		"Content-Type": "application/json",
	}
	resp, err = ctx.HTTP().Send(req)
	if err != nil {
		fmt.Printf("[SFN] execute http GET request with header error: %s\n", err)
		return
	}
	display(req, resp)
	// send http GET request with query
	req.Method = "GET"
	req.URL = host + "/get?name=foo&age=10"
	req.Body = []byte("hello world")
	req.Header = map[string]string{
		"Content-Type": "application/json",
	}
	resp, err = ctx.HTTP().Send(req)
	if err != nil {
		fmt.Printf("[SFN] execute http GET request with query error: %s\n", err)
		return
	}
	display(req, resp)
	// send http POST request
	req.Method = "POST"
	req.URL = host + "/post"
	req.Body = []byte("hello world")
	resp, err = ctx.HTTP().Send(req)
	if err != nil {
		fmt.Printf("[SFN] execute http POST request error: %s\n", err)
		return
	}
	display(req, resp)
	// send http PUT request
	req.Method = "PUT"
	req.URL = host + "/put"
	req.Body = []byte("hello world")
	resp, err = ctx.HTTP().Send(req)
	if err != nil {
		fmt.Printf("[SFN] execute http PUT request error: %s\n", err)
		return
	}
	display(req, resp)
	// send http DELETE request
	req.Method = "DELETE"
	req.URL = host + "/delete"
	req.Body = []byte("hello world")
	resp, err = ctx.HTTP().Send(req)
	if err != nil {
		fmt.Printf("[SFN] execute http DELETE request error: %s\n", err)
		return
	}
	display(req, resp)
	// send http PATCH request
	req.Method = "PATCH"
	req.URL = host + "/patch"
	req.Body = []byte("hello world")
	resp, err = ctx.HTTP().Send(req)
	if err != nil {
		fmt.Printf("[SFN] execute http PATCH request error: %s\n", err)
		return
	}
	display(req, resp)
	// dump output data
	// ctx.Write(0x34, []byte(output))
	log.Println("[SFN] -------------------------- END --------------------------")
}

func display(req *serverless.HTTPRequest, resp *serverless.HTTPResponse) {
	log.Printf("[SFN] execute [%s]`%s` response result: status=%v, status_code=%d, header=%+v\n",
		req.Method, req.URL,
		resp.Status, resp.StatusCode, resp.Header,
	)
	log.Printf("\t\tbody: %s\n", string(resp.Body))
}

func DataTags() []uint32 {
	return []uint32{0x33}
}
