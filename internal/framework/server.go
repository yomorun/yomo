package framework

import (
	"io"

	"github.com/yomorun/yomo/pkg/util"
)

type YomoFrameworkStream struct {
	Writer YomoFrameworkStreamWriter
	Reader YomoFrameworkStreamReader
}

type YomoFrameworkStreamWriter struct {
	Name string
	io.Writer
}

type YomoFrameworkStreamReader struct {
	Name string
	io.Reader
}

func (w YomoFrameworkStreamWriter) Write(b []byte) (int, error) {
	_, err := w.Writer.Write(b)
	return len(b), err
}

func (r YomoFrameworkStreamReader) Read(b []byte) (int, error) {
	return r.Reader.Read(b)
}

func NewServer(endpoint string, writer io.Writer, reader io.Reader) {
	util.QuicServer(endpoint, writer, reader)
}
