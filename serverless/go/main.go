package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
)

type RequestHeaders struct {
	Stream  bool   `json:"stream"`
	SfnName string `json:"sfn_name"`
	TraceID string `json:"trace_id"`
	ReqID   string `json:"req_id"`
}

type Request struct {
	Args    string `json:"args"`
	Context string `json:"context"`
}

type Response struct {
	Data  string `json:"data"`
	Error string `json:"error,omitempty"`
}

func readPacket[T any](r io.Reader) (*T, error) {
	// read 4 bytes from the connection, convert to uint32 format
	buf := make([]byte, 4)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint32(buf)

	// read the actual data packet
	buf = make([]byte, length)
	_, err = io.ReadFull(r, buf)
	if err != nil {
		return nil, err
	}

	// deserialize data
	var packet T
	err = json.Unmarshal(buf, &packet)
	if err != nil {
		return nil, err
	}

	return &packet, nil
}

func writePacket(w io.Writer, packet any) error {
	// Serialize data
	buf, err := json.Marshal(packet)
	if err != nil {
		return err
	}

	// write data length
	lengthBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBuf, uint32(len(buf)))
	_, err = w.Write(lengthBuf)
	if err != nil {
		return err
	}

	// write data bytes
	_, err = w.Write(buf)
	if err != nil {
		return err
	}

	return nil
}

func handleStream(stream io.ReadWriteCloser) {
	defer stream.Close()

	headers, err := readPacket[RequestHeaders](stream)
	if err != nil {
		log.Println("read headers error:", err)
		return
	}

	fmt.Println(headers)

	request, err := readPacket[Request](stream)
	if err != nil {
		log.Println("read request error:", err)
		return
	}

	if headers.Stream {
		ch := make(chan string)
		go func(ch <-chan string) {
			for x := range ch {
				err = writePacket(stream, &Response{Data: x})
				if err != nil {
					log.Println("write chunk error:", err)
				}
			}
		}(ch)

		err := StreamHandler(request.Args, ch)
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
		result, err := SimpleHandler(request.Args)
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
	listener, err := net.Listen("tcp", "127.0.0.1:0")
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
