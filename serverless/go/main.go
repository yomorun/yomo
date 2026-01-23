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
	TraceID string            `json:"trace_id"`
	ReqID   string            `json:"req_id"`
	SfnName string            `json:"sfn_name"`
	Stream  bool              `json:"stream"`
	Extra   map[string]string `json:"extra"`
}

type RequestBody struct {
	Args    string `json:"args"`
	Context string `json:"context"`
}

type Response struct {
	Data  string `json:"data"`
	Error string `json:"error,omitempty"`
}

func readPacket[T any](r io.Reader) (*T, error) {
	// read uint32 as length of the packet
	lengthBuf := make([]byte, 4)
	_, err := r.Read(lengthBuf)
	if err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint32(lengthBuf)

	// read the actual packet data
	packetData := make([]byte, length)
	_, err = r.Read(packetData)
	if err != nil {
		return nil, err
	}

	// decode the packet
	var packet T
	err = json.Unmarshal(packetData, &packet)
	if err != nil {
		return nil, err
	}

	return &packet, nil
}

func writePacket(w io.Writer, packet any) error {
	// encode the packet
	buf, err := json.Marshal(packet)
	if err != nil {
		return err
	}

	// write uint32 as length of the packet
	lengthBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBuf, uint32(len(buf)))
	_, err = w.Write(lengthBuf)
	if err != nil {
		return err
	}

	// write the actual packet data
	_, err = w.Write(buf)
	if err != nil {
		return err
	}

	return nil
}

func callSimpleHandler[ARGS any, RES any, CONTEXT any](
	handler func(ARGS, CONTEXT) (RES, error), rawArgs string, rawContext string,
) Response {
	var args ARGS
	if len(rawArgs) > 0 {
		err := json.Unmarshal([]byte(rawArgs), &args)
		if err != nil {
			return Response{Error: err.Error()}
		}
	}

	var context CONTEXT
	if len(rawContext) > 0 {
		err := json.Unmarshal([]byte(rawContext), &context)
		if err != nil {
			return Response{Error: err.Error()}
		}
	}

	res, err := handler(args, context)
	if err != nil {
		return Response{Error: err.Error()}
	}

	buf, err := json.Marshal(res)
	if err != nil {
		return Response{Error: err.Error()}
	}

	return Response{Data: string(buf)}
}

func callStreamHandler[ARGS any, RES any, CONTEXT any](
	handler func(ARGS, CONTEXT, chan<- RES) error, rawArgs string, rawContext string, rawCh chan<- Response,
) {
	defer close(rawCh)

	var args ARGS
	if len(rawArgs) > 0 {
		err := json.Unmarshal([]byte(rawArgs), &args)
		if err != nil {
			rawCh <- Response{Error: err.Error()}
			return
		}
	}

	var context CONTEXT
	if len(rawContext) > 0 {
		err := json.Unmarshal([]byte(rawContext), &context)
		if err != nil {
			rawCh <- Response{Error: err.Error()}
			return
		}
	}

	ch := make(chan RES)

	go func() {
		defer close(ch)

		err := handler(args, context, ch)
		if err != nil {
			rawCh <- Response{Error: err.Error()}
			return
		}
	}()

	for x := range ch {
		buf, err := json.Marshal(x)
		if err != nil {
			rawCh <- Response{Error: err.Error()}
			return
		}

		rawCh <- Response{Data: string(buf)}
	}
}

func handleStream(stream io.ReadWriteCloser) {
	defer stream.Close()

	headers, err := readPacket[RequestHeaders](stream)
	if err != nil {
		log.Println("read request header error:", err)
		return
	}

	body, err := readPacket[RequestBody](stream)
	if err != nil {
		log.Println("read request body error:", err)
		return
	} else if body == nil {
		log.Println("request body is nil")
		return
	}

	if headers.Stream {
		ch := make(chan Response)

		go callStreamHandler(StreamHandler, body.Args, body.Context, ch)

		for x := range ch {
			err = writePacket(stream, &x)
			if err != nil {
				log.Println("write chunk error:", err)
				return
			}
		}
	} else {
		response := callSimpleHandler(SimpleHandler, body.Args, body.Context)

		err = writePacket(stream, &response)
		if err != nil {
			log.Println("write response packet error:", err)
			return
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

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("accept error:", err)
			return
		}

		go handleStream(conn)
	}
}
