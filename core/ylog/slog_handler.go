// Package ylog provides handler that supports spliting log stream to common log stream and error log stream.
package ylog

import (
	"bytes"
	"io"
	"os"
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
	buf := newAsyncBuffer(0)

	h := bufferedSlogHandler(
		buf,
		conf.Format,
		parseToSlogLevel(conf.Level),
		conf.Verbose,
		conf.DisableTime,
	)

	return &handler{
		Handler:   h,
		buf:       buf,
		writer:    parseToWriter(conf, conf.Output, os.Stdout),
		errWriter: parseToWriter(conf, conf.ErrorOutput, os.Stderr),
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

	if r.Level == slog.LevelError {
		_, err = io.Copy(h.errWriter, h.buf)
	} else {
		_, err = io.Copy(h.writer, h.buf)
	}
	h.buf.Reset()

	return err
}

func (h *handler) WithAttrs(as []slog.Attr) slog.Handler {
	return &handler{
		buf:       h.buf,
		writer:    h.writer,
		errWriter: h.errWriter,
		Handler:   h.Handler.WithAttrs(as),
	}
}

func (h *handler) WithGroup(name string) slog.Handler {
	return &handler{
		buf:       h.buf,
		writer:    h.writer,
		errWriter: h.errWriter,
		Handler:   h.Handler.WithGroup(name),
	}
}

func bufferedSlogHandler(buf io.Writer, format string, level slog.Level, verbose, disableTime bool) slog.Handler {
	opt := slog.HandlerOptions{
		AddSource: verbose,
		Level:     level,
	}
	if disableTime {
		opt.ReplaceAttr = func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == "time" {
				return slog.Attr{}
			}
			return a
		}
	}

	var h slog.Handler
	if strings.ToLower(format) == "json" {
		h = opt.NewJSONHandler(buf)
	} else {
		h = opt.NewTextHandler(buf)
	}

	return h
}
