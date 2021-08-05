package quic

import (
	"bytes"
	"fmt"
	"io"

	"github.com/lucas-clemente/quic-go"
)

// Stream is a bidirectional QUIC stream.
type Stream interface {
	quic.Stream
}

// ReceiveStream is a unidirectional Receive stream.
type ReceiveStream interface {
	quic.ReceiveStream
}

// SendStream is a unidirectional Send stream.
type SendStream interface {
	quic.SendStream
}

// Session is the QUIC session.
type Session interface {
	quic.Session
}

const bufferSize = 1024 // bufferSize is the size of buffer when receiving data from QUIC Stream.

func ReadStream(stream io.Reader) ([]byte, error) {
	b := &bytes.Buffer{}

LOOP_READ_STREAM:
	for {
		buf := make([]byte, bufferSize)
		n, err := stream.Read(buf)
		// read all data.
		if err == io.EOF {
			fmt.Println("eof")
			break LOOP_READ_STREAM
		}

		// read data failed.
		if err != nil {
			return nil, err
		}

		// reading data
		b.Write(buf[:n])
		fmt.Println(n)
	}

	return b.Bytes(), nil
}
