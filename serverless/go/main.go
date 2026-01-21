package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
)

type Request struct {
	Args   string `json:"args"`
	Stream bool   `json:"stream"`
}

type Response struct {
	Result string `json:"result"`
	Error  string `json:"error"`
}

type Chunk struct {
	Chunk string `json:"chunk"`
}

type ChunkDone struct {
	Error string `json:"error"`
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

	packet, err := readPacket[Request](stream)
	if err != nil {
		log.Println("read request packet error:", err)
		return
	}

	if packet.Stream {
		ch := make(chan string)
		go func(ch <-chan string) {
			for x := range ch {
				err = writePacket(stream, &Chunk{Chunk: x})
				if err != nil {
					log.Println("write chunk packet error:", err)
					return
				}
			}
		}(ch)

		StreamHandler(packet.Args, ch)
	} else {
		result, err := SimpleHandler(packet.Args)
		response := Response{Result: result}
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
