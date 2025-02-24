package ai

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

// EventResponseWriter is the interface for writing events to the underlying ResponseWriter.
type EventResponseWriter interface {
	// EventResponseWriter should implement http.ResponseWriter
	http.ResponseWriter
	// EventResponseWriter should implement http.Flusher
	http.Flusher
	// EventResponseWriter should implement StreamRecorder
	StreamRecorder
	// SetStreamHeader sets the stream headers of the underlying ResponseWriter.
	SetStreamHeader() http.Header
	// WriteStreamEvent writes the event to the underlying ResponseWriter.
	WriteStreamEvent(any) error
	// WriteStreamDone writes the done event to the underlying ResponseWriter.
	WriteStreamDone() error
}

// StreamRecorder records the stream status of the ResponseWriter.
type StreamRecorder interface {
	// RecordIsStream records the stream status of the ResponseWriter.
	RecordIsStream(bool)
	// IsStream returns the stream status of the ResponseWriter.
	IsStream() bool
	// RecordError records the error of the request.
	RecordError(error)
	// GetError returns the error of the request.
	GetError() error
	// RecordTTFT records the TTFT(Time to First Token) of the request.
	RecordTTFT(time.Time)
	// GetTTFT returns the TTFT(Time to First Token) of the request.
	GetTTFT() time.Time
}

var _ EventResponseWriter = (*responseWriter)(nil)

// responseWriter is a wrapper for http.ResponseWriter.
// It is used to add TTFT and Err to the response.
type responseWriter struct {
	recorder
	underlying http.ResponseWriter
}

// NewResponseWriter returns a new ResponseWriter.
func NewResponseWriter(w http.ResponseWriter) EventResponseWriter {
	return &responseWriter{
		underlying: w,
	}
}

// SetStreamHeader sets the stream headers of the underlying ResponseWriter.
func (w *responseWriter) SetStreamHeader() http.Header {
	return SetStreamHeader(w.underlying)
}

// WriteStreamEvent writes the event to the underlying ResponseWriter follow the OpenAI API spec.
func (w *responseWriter) WriteStreamEvent(e any) error {
	switch event := e.(type) {
	case openai.ChatCompletionStreamResponse:
		if _, err := io.WriteString(w, "data: "); err != nil {
			return err
		}
		if err := json.NewEncoder(w).Encode(event); err != nil {
			return err
		}
		if _, err := io.WriteString(w, "\n"); err != nil {
			return err
		}
		w.Flush()
	case []openai.ToolCall:
		// do nothing here to keep the responseWriter consistent with the OpenAI API spec.
	case []ToolCallResult:
		// do nothing here to keep the responseWriter consistent with the OpenAI API spec.
	}
	return nil
}

func (w *responseWriter) WriteStreamDone() error {
	_, err := io.WriteString(w, "data: [DONE]")
	w.Flush()

	return err
}

func (w *responseWriter) Header() http.Header         { return w.underlying.Header() }
func (w *responseWriter) Write(b []byte) (int, error) { return w.underlying.Write(b) }
func (w *responseWriter) WriteHeader(code int)        { w.underlying.WriteHeader(code) }

func (w *responseWriter) Flush() {
	flusher, ok := w.underlying.(http.Flusher)
	if ok {
		flusher.Flush()
	}
}

type recorder struct {
	isStream bool
	err      error
	ttft     time.Time
}

// NewStreamRecorder returns a new StreamRecorder.
func NewStreamRecorder() StreamRecorder {
	return &recorder{}
}

func (w *recorder) GetError() error              { return w.err }
func (w *recorder) GetTTFT() time.Time           { return w.ttft }
func (w *recorder) IsStream() bool               { return w.isStream }
func (w *recorder) RecordError(err error)        { w.err = err }
func (w *recorder) RecordIsStream(isStream bool) { w.isStream = isStream }
func (w *recorder) RecordTTFT(t time.Time)       { w.ttft = t }

func SetStreamHeader(w http.ResponseWriter) http.Header {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache, must-revalidate")
	h.Set("x-content-type-options", "nosniff")
	return h
}
