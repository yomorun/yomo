// package provides handler that supports spliting log stream to common log stream and error log stream.
package ylog

import (
	"bytes"
	"io"
	"strings"
	"sync"

	"golang.org/x/exp/slog"
)

// handler supports spliting log stream to common log stream and error log stream.
type handler struct {
	slog.Handler

	buf *asyncBuffer

	writer    io.Writer
	errWriter io.Writer
}

type asyncBuffer struct {
	sync.Mutex
	underlying *bytes.Buffer
}

func newAsyncBuffer(cap int) *asyncBuffer {
	return &asyncBuffer{
		underlying: bytes.NewBuffer(make([]byte, cap)),
	}
}

func (buf *asyncBuffer) Write(b []byte) (int, error) {
	buf.Lock()
	defer buf.Unlock()

	return buf.underlying.Write(b)
}

func (buf *asyncBuffer) Read(p []byte) (int, error) {
	buf.Lock()
	defer buf.Unlock()

	return buf.underlying.Read(p)
}

func (buf *asyncBuffer) Reset() {
	buf.Lock()
	defer buf.Unlock()

	buf.underlying.Reset()
}

// NewHandlerFromConfig creates a slog.Handler from conf
func NewHandlerFromConfig(conf Config) slog.Handler {
	buf := newAsyncBuffer(256)

	h := bufferedSlogHandler(buf, conf.Format, conf.DebugMode)

	h.Enabled(parseToSlogLevel(conf.Level))

	return &handler{
		Handler:   h,
		buf:       buf,
		writer:    mustParseToWriter(conf.Output),
		errWriter: mustParseToWriter(conf.ErrorOutput),
	}
}

func (h *handler) Enabled(level slog.Level) bool {
	return h.Handler.Enabled(level)
}

func (h *handler) Handle(r slog.Record) error {
	err := h.Handler.Handle(r)
	if err != nil {
		return err
	}

	if r.Level == slog.ErrorLevel {
		_, err = io.Copy(h.errWriter, h.buf)
	}
	h.buf.Reset()

	return err
}

func (h *handler) WithAttrs(as []slog.Attr) slog.Handler {
	return &handler{
		buf:       h.buf,
		errWriter: h.errWriter,
		Handler:   h.Handler.WithAttrs(as),
	}
}

func (h *handler) WithGroup(name string) slog.Handler {
	return &handler{
		buf:       h.buf,
		errWriter: h.errWriter,
		Handler:   h.Handler.WithGroup(name),
	}
}

func bufferedSlogHandler(buf io.Writer, format string, debugMode bool) slog.Handler {
	opt := slog.HandlerOptions{
		AddSource: debugMode,
	}

	var h slog.Handler
	if strings.ToLower(format) == "json" {
		h = opt.NewJSONHandler(buf)
	} else {
		h = opt.NewTextHandler(buf)
	}

	return h
}
