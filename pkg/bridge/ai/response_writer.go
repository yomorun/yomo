package ai

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"time"

	openai "github.com/yomorun/go-openai"
	"github.com/yomorun/yomo/ai"
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
	// InterceptError intercepts the error and returns the error body for responding.
	InterceptError(code int, err error) (int, ErrorResponseBody)
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
}

var _ EventResponseWriter = (*responseWriter)(nil)

// responseWriter is a wrapper for http.ResponseWriter.
// It is used to add TTFT and Err to the response.
type responseWriter struct {
	logger *slog.Logger
	recorder
	underlying http.ResponseWriter
}

// NewResponseWriter returns a new ResponseWriter.
func NewResponseWriter(w http.ResponseWriter, logger *slog.Logger) EventResponseWriter {
	return &responseWriter{
		logger:     logger,
		underlying: w,
	}
}

// SetStreamHeader sets the stream headers of the underlying ResponseWriter.
func (w *responseWriter) SetStreamHeader() http.Header {
	return SetStreamHeader(w.underlying)
}

// WriteError writes an error to the underlying ResponseWriter.
func (w *responseWriter) InterceptError(code int, err error) (int, ErrorResponseBody) {
	pcode, codeString, errString := parseCodeError(err)

	w.logger.Error("bridge server error", "err", errString, "err_type", reflect.TypeOf(err).String())

	if pcode == 0 {
		return code, ErrorResponseBody{
			Code:    http.StatusText(code),
			Message: err.Error(),
		}
	}
	return pcode, ErrorResponseBody{
		Code:    codeString,
		Message: errString,
	}
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
		w.logger.Debug("tool calls", "tool_calls", event)
	case []ai.ToolCallResult:
		w.logger.Debug("tool results", "tool_results", event)
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

// ErrorResponse is the response for error, almost follow the OpenAI API spec.
type ErrorResponse struct {
	Error ErrorResponseBody `json:"error"`
}

// ErrorResponseBody is the main content for ErrorResponse.
type ErrorResponseBody struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// Error implements the error interface for ErrorResponseBody.
func (e ErrorResponseBody) Error() string {
	return e.Message
}

// parseCodeError returns the status code, error code string and error message string.
func parseCodeError(err error) (code int, codeString string, message string) {
	switch e := err.(type) {
	// bad request
	case *json.SyntaxError:
		return http.StatusBadRequest, "invalid_request_error", fmt.Sprintf("Invalid request: %s", e.Error())
	case *json.UnmarshalTypeError:
		return http.StatusBadRequest, "invalid_request_error", fmt.Sprintf("Invalid type for `%s`: expected a %s, but got a %s", e.Field, e.Type.String(), e.Value)

	case *openai.APIError:
		// handle azure api error
		if e.InnerError != nil {
			return e.HTTPStatusCode, e.InnerError.Code, e.Message
		}
		// handle openai api error
		eCode, ok := e.Code.(string)
		if ok {
			return e.HTTPStatusCode, eCode, e.Message
		}
		codeString = e.Type
		return

	case *openai.RequestError:
		return e.HTTPStatusCode, e.HTTPStatus, string(e.Body)
	}

	return code, reflect.TypeOf(err).Name(), err.Error()
}
