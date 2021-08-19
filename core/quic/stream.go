package quic

import (
	"bytes"
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

// ReadStream read data from QUIC stream.
func ReadStream(stream io.Reader) ([]byte, error) {
	b := &bytes.Buffer{}

LOOP_READ_STREAM:
	for {
		buf := make([]byte, bufferSize)
		n, err := stream.Read(buf)

		// read data failed.
		if err != nil && err != io.EOF {
			return nil, err
		}

		// read all data.
		if err == io.EOF && n == 0 {
			break LOOP_READ_STREAM
		}

		// reading data
		b.Write(buf[:n])
	}

	return b.Bytes(), nil
}
