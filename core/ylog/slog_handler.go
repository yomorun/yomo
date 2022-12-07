package ylog

import (
	"bytes"
	"io"
	"strings"
	"sync"

	"golang.org/x/exp/slog"
)

type handler struct {
	slog.Handler

	mu  sync.Mutex
	buf *bytes.Buffer

	writer    io.Writer
	errWriter io.Writer
}

func NewHandlerFromConfig(conf Config) slog.Handler {
	buf := bytes.NewBuffer(make([]byte, 256))

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

	h.mu.Lock()
	defer h.mu.Unlock()
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

func bufferedSlogHandler(buf *bytes.Buffer, format string, debugMode bool) slog.Handler {
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
