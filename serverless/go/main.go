package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
)

type RequestHeaders struct {
	TraceID   string `json:"trace_id"`
	ReqeustID string `json:"request_id"`
	SfnName   string `json:"sfn_name"`
	Extension string `json:"extension"`
}

type RequestBody[ARGS any, CONTEXT any] struct {
	Args    ARGS    `json:"args"`
	Context CONTEXT `json:"context"`
}

type ResponseHeaders struct {
	StatusCode uint16 `json:"status_code"`
	ErrorMsg   string `json:"error_msg"`
	Stream     bool   `json:"stream"`
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
	_, err := r.Read(lengthBuf)
	if err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint32(lengthBuf)

	// read the actual data
	data := make([]byte, length)
	_, err = r.Read(data)
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

func callSimpleHandler[ARGS any, RES any, CONTEXT any](
	handler func(ARGS, CONTEXT) (RES, error), body []byte,
) ([]byte, error) {
	var request RequestBody[ARGS, CONTEXT]
	err := json.Unmarshal(body, &request)
	if err != nil {
		return nil, err
	}

	res, err := handler(request.Args, request.Context)
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

func callStreamHandler[ARGS any, RES any, CONTEXT any](
	stream io.Writer, handler func(ARGS, CONTEXT, chan<- RES) error, body []byte,
) error {
	var request RequestBody[ARGS, CONTEXT]
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

	err = handler(request.Args, request.Context, ch)
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

	reqBody, err := readBytes(stream)
	if err != nil {
		log.Println("read request body error:", err)
		return
	}
	log.Printf("req body: %s\n", string(reqBody))

	// todo: parse serverless handler stream mode according to go ast
	resHeaders := ResponseHeaders{
		StatusCode: 200,
		Stream:     len(reqBody) > 50,
	}

	if resHeaders.Stream {
		log.Println("call stream handler")

		writePacket(stream, &resHeaders)

		err := callStreamHandler(stream, StreamHandler, reqBody)
		if err != nil {
			log.Println("call stream handler error:", err)

			writePacket(stream, &Chunk{Error: err.Error()})
		}
	} else {
		log.Println("call simple handler")

		resBody, err := callSimpleHandler(SimpleHandler, reqBody)
		if err != nil {
			log.Println("call simple handler error:", err)

			resHeaders.StatusCode = 500
			resHeaders.ErrorMsg = err.Error()
		}
		log.Printf("simple handler response: %s\n", string(resBody))

		writePacket(stream, &resHeaders)

		if err == nil {
			writeBytes(stream, resBody)
		}
	}
}

func main() {
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
			log.Println("accept error:", err)
			return
		}

		go handleStream(conn)
	}
}
