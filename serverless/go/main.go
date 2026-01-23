package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
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

type Request struct {
	Headers RequestHeaders `json:"headers"`
	Body    RequestBody    `json:"body"`
}

type Response struct {
	Data  string `json:"data"`
	Error string `json:"error,omitempty"`
}

func readPacket[T any](r io.Reader) (*T, error) {
	buf, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	var packet T
	err = json.Unmarshal(buf, &packet)
	if err != nil {
		return nil, err
	}

	return &packet, nil
}

func writePacket(w io.Writer, packet any) error {
	buf, err := json.Marshal(packet)
	if err != nil {
		return err
	}

	_, err = w.Write(buf)
	if err != nil {
		return err
	}

	_, err = w.Write([]byte{'\n'})
	if err != nil {
		return err
	}

	return nil
}

func handleStream(stream io.ReadWriteCloser) {
	defer stream.Close()

	request, err := readPacket[Request](stream)
	if err != nil {
		log.Println("read request error:", err)
		return
	}

	if request.Headers.Stream {
		ch := make(chan string)
		go func(ch <-chan string) {
			for x := range ch {
				err = writePacket(stream, &Response{Data: x})
				if err != nil {
					log.Println("write chunk error:", err)
				}
			}
		}(ch)

		err := StreamHandler(request.Body.Args, ch)
		if err != nil {
			log.Println("stream handler error:", err)

			err = writePacket(stream, &Response{Error: err.Error()})
			if err != nil {
				log.Println("write chunk error:", err)
			}

			return
		}

		close(ch)
	} else {
		result, err := SimpleHandler(request.Body.Args)
		response := Response{Data: result}
		if err != nil {
			response.Error = err.Error()
		}

		err = writePacket(stream, &response)
		if err != nil {
			log.Println("write response packet error:", err)
			return
		}
	}
}

func main() {
	listener, err := net.Listen("tcp", "127.0.0.1:12000")
	if err != nil {
		log.Println("listen error:", err)
		return
	}
	defer listener.Close()

	fmt.Println(listener.Addr().String())

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("accept error:", err)
			return
		}

		log.Println("new connection:", conn.RemoteAddr().String())

		go handleStream(conn)
	}
}
