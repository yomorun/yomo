package ai

import (
	"encoding/json"
	"io"
	"net/http"
	"time"
)

// ResponseWriter is a wrapper for http.ResponseWriter.
// It is used to add TTFT and Err to the response.
type ResponseWriter struct {
	IsStream   bool
	Err        error
	TTFT       time.Time
	underlying http.ResponseWriter
}

// NewResponseWriter returns a new ResponseWriter.
func NewResponseWriter(w http.ResponseWriter) *ResponseWriter {
	return &ResponseWriter{
		underlying: w,
	}
}

// Header returns the headers of the underlying ResponseWriter.
func (w *ResponseWriter) Header() http.Header {
	return w.underlying.Header()
}

// Write writes the data to the underlying ResponseWriter.
func (w *ResponseWriter) Write(b []byte) (int, error) {
	return w.underlying.Write(b)
}

// WriteHeader writes the header to the underlying ResponseWriter.
func (w *ResponseWriter) WriteHeader(code int) {
	w.underlying.WriteHeader(code)
}

// WriteStreamEvent writes the event to the underlying ResponseWriter.
func (w *ResponseWriter) WriteStreamEvent(event any) error {
	if _, err := io.WriteString(w, "data: "); err != nil {
		return err
	}
	if err := json.NewEncoder(w).Encode(event); err != nil {
		return err
	}
	if _, err := io.WriteString(w, "\n"); err != nil {
		return err
	}
	flusher, ok := w.underlying.(http.Flusher)
	if ok {
		flusher.Flush()
	}
	return nil
}

// WriteStreamDone writes the done event to the underlying ResponseWriter.
func (w *ResponseWriter) WriteStreamDone() error {
	_, err := io.WriteString(w, "data: [DONE]")

	flusher, ok := w.underlying.(http.Flusher)
	if ok {
		flusher.Flush()
	}

	return err
}

// SetStreamHeader sets the stream headers of the underlying ResponseWriter.
func (w *ResponseWriter) SetStreamHeader() http.Header {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache, must-revalidate")
	h.Set("x-content-type-options", "nosniff")
	return h
}

// Flush flushes the underlying ResponseWriter.
func (w *ResponseWriter) Flush() {
	flusher, ok := w.underlying.(http.Flusher)
	if ok {
		flusher.Flush()
	}
}
