package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"reflect"
)

var (
	simpleHandler func(Arguments) (Result, error)       = nil
	streamHandler func(Arguments, chan<- Result) error  = nil
	rawHandler    func(io.Reader, io.WriteCloser) error = nil

	handlerMode string
)

type RequestHeaders struct {
	SfnName    string `json:"sfn_name"`
	TraceID    string `json:"trace_id"`
	ReqeustID  string `json:"request_id"`
	BodyFormat string `json:"body_format"`
	Extension  string `json:"extension"`
}

type RequestBody[ARGS any] struct {
	Args ARGS `json:"args"`
	// todo: parse serverless context
	// Context CONTEXT `json:"context"`
}

type ResponseHeaders struct {
	StatusCode uint16 `json:"status_code"`
	ErrorMsg   string `json:"error_msg"`
	BodyFormat string `json:"body_format"`
	Extension  string `json:"extension"`
}

type ResponseBody struct {
	Data any `json:"data"`
}

type Chunk struct {
	Data  any    `json:"data"`
	Error string `json:"error,omitempty"`
}

func readBytes(r io.Reader) ([]byte, error) {
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

func writeBytes(w io.Writer, data []byte) {
	// write uint32 as length of the data
	lengthBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBuf, uint32(len(data)))
	w.Write(lengthBuf)

	// write the actual data
	w.Write(data)
}

func readPacket[T any](r io.Reader) (*T, error) {
	buf, err := readBytes(r)
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

func writePacket(w io.Writer, packet any) {
	// encode the packet
	buf, _ := json.Marshal(packet)

	writeBytes(w, buf)
}

func callSimpleHandler[ARGS any, RES any](
	handler func(ARGS) (RES, error), body []byte,
) ([]byte, error) {
	var request RequestBody[ARGS]
	err := json.Unmarshal(body, &request)
	if err != nil {
		return nil, err
	}

	res, err := handler(request.Args)
	if err != nil {
		return nil, err
	}

	response := ResponseBody{Data: res}
	buf, err := json.Marshal(response)
	if err != nil {
		return nil, err
	}

	return buf, nil
}

func callStreamHandler[ARGS any, RES any](
	stream io.Writer, handler func(ARGS, chan<- RES) error, body []byte,
) error {
	var request RequestBody[ARGS]
	err := json.Unmarshal(body, &request)
	if err != nil {
		return err
	}

	ch := make(chan RES)
	defer close(ch)

	go func() {
		for x := range ch {
			writePacket(stream, &Chunk{Data: x})
		}
	}()

	err = handler(request.Args, ch)
	if err != nil {
		return err
	}

	return nil
}

func handleStream(stream io.ReadWriteCloser) {
	defer stream.Close()

	reqHeaders, err := readPacket[RequestHeaders](stream)
	if err != nil {
		log.Println("read request header error:", err)
		return
	}
	log.Printf("req headers: trace_id=%s, request_id=%s\n", reqHeaders.TraceID, reqHeaders.ReqeustID)

	// todo: read description, set serverless context

	resHeaders := ResponseHeaders{
		StatusCode: 200,
	}

	switch handlerMode {
	case "simple":
		log.Println("call simple handler")

		reqBody, err := readBytes(stream)
		if err != nil {
			log.Println("read request body error:", err)
			return
		}
		log.Printf("req body: %s\n", string(reqBody))

		resBody, err := callSimpleHandler(simpleHandler, reqBody)
		if err != nil {
			log.Println("call simple handler error:", err)

			resHeaders.BodyFormat = "null"
			resHeaders.StatusCode = 500
			resHeaders.ErrorMsg = err.Error()
		} else {
			log.Printf("simple handler response: %s\n", string(resBody))

			resHeaders.BodyFormat = "bytes"
		}

		writePacket(stream, &resHeaders)

		if err == nil {
			writeBytes(stream, resBody)
		}
	case "stream":
		log.Println("call stream handler")

		reqBody, err := readBytes(stream)
		if err != nil {
			log.Println("read request body error:", err)
			return
		}
		log.Printf("req body: %s\n", string(reqBody))

		resHeaders.BodyFormat = "chunk"
		writePacket(stream, &resHeaders)

		err = callStreamHandler(stream, streamHandler, reqBody)
		if err != nil {
			log.Println("call stream handler error:", err)

			writePacket(stream, &Chunk{Error: err.Error()})
		}
	case "raw":
		log.Println("call raw handler")

		resHeaders.BodyFormat = "bytes"
		writePacket(stream, &resHeaders)

		err := rawHandler(stream, stream)
		if err != nil {
			log.Println("call raw handler error:", err)
		}
	default:
		resHeaders.StatusCode = 500
		resHeaders.ErrorMsg = "unimplemented serverless mode: " + handlerMode
	}
}

// reflect application Handler function
func reflectApp() (string, error) {
	h := reflect.ValueOf(Handler)

	switch h.Type() {
	case reflect.TypeOf(simpleHandler):
		reflect.ValueOf(&simpleHandler).Elem().Set(h)
		return "simple", nil
	case reflect.TypeOf(streamHandler):
		reflect.ValueOf(&streamHandler).Elem().Set(h)
		return "stream", nil
	case reflect.TypeOf(rawHandler):
		reflect.ValueOf(&rawHandler).Elem().Set(h)
		return "raw", nil
	default:
		return "", fmt.Errorf("unsupported handler type: %s", h.Type())
	}
}

func main() {
	mode, err := reflectApp()
	if err != nil {
		os.Exit(1)
	}
	handlerMode = mode

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		os.Exit(1)
	}
	defer listener.Close()

	fmt.Println(listener.Addr().String())

	log.Println("serverless handler mode:", handlerMode)

	go func() {
		for {
			io.ReadAll(os.Stdin)
			os.Exit(0)
		}
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("accept error:", err)
			return
		}

		go handleStream(conn)
	}
}
