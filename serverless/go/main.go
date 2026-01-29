package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"reflect"
)

var (
	yomoSimpleHandler func(Arguments) (Result, error) = nil
	yomoStreamHandler func(Arguments, chan<- Result)  = nil
)

type YomoRequestHeaders struct {
	SfnName    string `json:"sfn_name"`
	TraceID    string `json:"trace_id"`
	ReqeustID  string `json:"request_id"`
	BodyFormat string `json:"body_format"`
	Extension  string `json:"extension"`
}

type YomoRequestBody struct {
	Args Arguments `json:"args"`
	// todo
	// Context map[string]any `json:"context"`
}

type YomoResponseHeaders struct {
	StatusCode uint16 `json:"status_code"`
	ErrorMsg   string `json:"error_msg"`
	BodyFormat string `json:"body_format"`
	Extension  string `json:"extension"`
}

type YomoResponseBody struct {
	Result   any    `json:"result"`
	ErrorMsg string `json:"error_msg,omitempty"`
}

type Chunk struct {
	Chunk any `json:"chunk"`
	// todo: error process
}

func yomoReadBytes(r io.Reader) ([]byte, error) {
	lengthBuf := make([]byte, 4)
	_, err := io.ReadFull(r, lengthBuf)
	if err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint32(lengthBuf)

	// read the actual data
	data := make([]byte, length)
	_, err = io.ReadFull(r, data)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func yomoWriteBytes(w io.Writer, data []byte) {
	// write uint32 as length of the data
	lengthBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBuf, uint32(len(data)))
	w.Write(lengthBuf)

	// write the actual data
	w.Write(data)
}

func yomoReadPacket[T any](r io.Reader) (*T, error) {
	buf, err := yomoReadBytes(r)
	if err != nil {
		return nil, err
	}

	// decode the packet
	var packet T
	err = json.Unmarshal(buf, &packet)
	if err != nil {
		return nil, err
	}

	return &packet, nil
}

func yomoWritePacket(w io.Writer, packet any) {
	// encode the packet
	buf, _ := json.Marshal(packet)

	yomoWriteBytes(w, buf)
}

func yomoHandleStream(handlerMode string, conn io.ReadWriteCloser) error {
	defer conn.Close()

	_, err := yomoReadPacket[YomoRequestHeaders](conn)
	if err != nil {
		yomoWritePacket(
			conn,
			&YomoResponseHeaders{
				StatusCode: 400,
				ErrorMsg:   err.Error(),
				BodyFormat: "null",
			},
		)
		return err
	}

	reqBody, err := yomoReadPacket[YomoRequestBody](conn)
	if err != nil {
		yomoWritePacket(
			conn,
			&YomoResponseHeaders{
				StatusCode: 400,
				ErrorMsg:   err.Error(),
				BodyFormat: "null",
			},
		)
		return err
	}

	switch handlerMode {
	case "simple":
		resBody := &YomoResponseBody{}
		result, err := yomoSimpleHandler(reqBody.Args)
		if err == nil {
			resBody.Result = result
		} else {
			resBody.ErrorMsg = err.Error()
		}

		yomoWritePacket(
			conn,
			&YomoResponseHeaders{
				StatusCode: 200,
				BodyFormat: "bytes",
			},
		)
		yomoWritePacket(conn, resBody)
	case "stream":
		yomoWritePacket(
			conn,
			&YomoResponseHeaders{
				StatusCode: 200,
				BodyFormat: "chunk",
			},
		)

		ch := make(chan Result)
		defer close(ch)

		go func() {
			for x := range ch {
				yomoWritePacket(conn, &Chunk{Chunk: x})
			}
		}()

		yomoStreamHandler(reqBody.Args, ch)
	default:
		err = fmt.Errorf("unimplemented serverless mode: %s", handlerMode)
		yomoWritePacket(
			conn,
			&YomoResponseHeaders{
				StatusCode: 500,
				ErrorMsg:   err.Error(),
				BodyFormat: "null",
			},
		)
		return err
	}

	return nil
}

// reflect application Handler function
func yomoReflectApp() (string, error) {
	h := reflect.ValueOf(Handler)

	switch h.Type() {
	case reflect.TypeOf(yomoSimpleHandler):
		reflect.ValueOf(&yomoSimpleHandler).Elem().Set(h)
		return "simple", nil
	case reflect.TypeOf(yomoStreamHandler):
		reflect.ValueOf(&yomoStreamHandler).Elem().Set(h)
		return "stream", nil
	default:
		return "", fmt.Errorf("unsupported handler type: %s", h.Type())
	}

	// todo: read description, set serverless context
}

func main() {
	handlerMode, err := yomoReflectApp()
	if err != nil {
		os.Exit(1)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		os.Exit(1)
	}
	defer listener.Close()

	fmt.Println(listener.Addr().String())

	go func() {
		for {
			io.ReadAll(os.Stdin)
			os.Exit(0)
		}
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			os.Exit(1)
		}

		go yomoHandleStream(handlerMode, conn)
	}
}
