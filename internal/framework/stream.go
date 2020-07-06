package framework

import (
	"io"

	"github.com/yomorun/yomo/pkg/plugin"

	txtkv "github.com/10cella/yomo-txtkv-codec"
)

type YomoStreamPluginStream struct {
	Writer YomoStreamPluginStreamWriter
	Reader YomoStreamPluginStreamReader
}

type YomoStreamPluginStreamWriter struct {
	Name   string
	Plugin plugin.YomoStreamPlugin
	io.Writer
}

type YomoStreamPluginStreamReader struct {
	Name string
	io.Reader
}

func (w YomoStreamPluginStreamWriter) Write(b []byte) (int, error) {
	head := b[:1]
	var err error = nil

	// stream
	for _, c := range b[1:] {
		if c == txtkv.GetEnd() {
			buf, _ := w.Plugin.HandleStream([]byte{}, true)
			buf = append(head, buf...)
			buf = append(buf, txtkv.GetEnd())
			_, err = w.Writer.Write(buf)
		} else {
			buf, _ := w.Plugin.HandleStream([]byte{c}, false)
			buf = append(head, buf...)
			_, err = w.Writer.Write(buf)
		}
	}

	return len(b), err
}

func (r YomoStreamPluginStreamReader) Read(b []byte) (int, error) {
	return r.Reader.Read(b)
}

func NewStreamPlugin(h plugin.YomoStreamPlugin) YomoStreamPluginStream {
	name := "plugin"
	reader, writer := io.Pipe()
	w := YomoStreamPluginStreamWriter{name, h, writer}
	r := YomoStreamPluginStreamReader{name, reader}
	s := YomoStreamPluginStream{Writer: w, Reader: r}
	return s
}
