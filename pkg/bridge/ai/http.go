package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"reflect"

	openai "github.com/sashabaranov/go-openai"
	"github.com/yomorun/yomo/ai"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

type callerContextKey struct{}

// WithCallerContext adds the caller to the request context
func WithCallerContext(ctx context.Context, caller *Caller) context.Context {
	return context.WithValue(ctx, callerContextKey{}, caller)
}

// FromCallerContext returns the caller from the request context
func FromCallerContext(ctx context.Context) *Caller {
	caller, ok := ctx.Value(callerContextKey{}).(*Caller)
	if !ok {
		return nil
	}
	return caller
}

type transIDContextKey struct{}

// WithTransIDContext adds the transID to the request context
func WithTransIDContext(ctx context.Context, transID string) context.Context {
	return context.WithValue(ctx, transIDContextKey{}, transID)
}

// FromTransIDContext returns the transID from the request context
func FromTransIDContext(ctx context.Context) string {
	val, ok := ctx.Value(transIDContextKey{}).(string)
	if !ok {
		return ""
	}
	return val
}

type tracerContextKey struct{}

// WithTracerContext adds the tracer to the request context
func WithTracerContext(ctx context.Context, tracer trace.Tracer) context.Context {
	return context.WithValue(ctx, tracerContextKey{}, tracer)
}

// FromTransIDContext returns the transID from the request context
func FromTracerContext(ctx context.Context) trace.Tracer {
	val, ok := ctx.Value(tracerContextKey{}).(trace.Tracer)
	if !ok {
		return new(noop.Tracer)
	}
	return val
}

// RespondWithError writes an error to response according to the OpenAI API spec.
func RespondWithError(w http.ResponseWriter, code int, err error, logger *slog.Logger) {
	code, errString := parseCodeError(code, err)
	logger.Error("bridge server error", "err", errString, "err_type", reflect.TypeOf(err).String())

	w.WriteHeader(code)
	w.Write([]byte(fmt.Sprintf(`{"error":{"code":"%d","message":"%s"}}`, code, errString)))
}

func parseCodeError(code int, err error) (int, string) {
	errString := err.Error()

	switch e := err.(type) {
	case *openai.APIError:
		code = e.HTTPStatusCode
		errString = e.Message
	case *openai.RequestError:
		code = e.HTTPStatusCode
		errString = e.Error()
	}

	return code, errString
}

// NewServeMux creates a new http.ServeMux for the llm bridge server.
func NewServeMux(h *Handler) *http.ServeMux {
	mux := http.NewServeMux()

	// GET /overview
	mux.HandleFunc("/overview", h.HandleOverview)
	// POST /invoke
	mux.HandleFunc("/invoke", h.HandleInvoke)
	// POST /v1/chat/completions (OpenAI compatible interface)
	mux.HandleFunc("/v1/chat/completions", h.HandleChatCompletions)

	return mux
}

// DecorateHandler decorates the http.Handler.
func DecorateHandler(h http.Handler, decorates ...func(handler http.Handler) http.Handler) http.Handler {
	// decorate the http.Handler
	for i := len(decorates) - 1; i >= 0; i-- {
		h = decorates[i](h)
	}
	return h
}

// Handler handles the http request.
type Handler struct {
	service *Service
}

// NewHandler return a hander that handles chat completions requests.
func NewHandler(service *Service) *Handler {
	return &Handler{service}
}

// HandleOverview is the handler for GET /overview
func (h *Handler) HandleOverview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	tools, err := ai.ListToolCalls(FromCallerContext(r.Context()).Metadata())
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, err, h.service.Logger())
		return
	}

	functions := make([]*openai.FunctionDefinition, len(tools))
	for i, tc := range tools {
		functions[i] = tc.Function
	}

	json.NewEncoder(w).Encode(&ai.OverviewResponse{Functions: functions})
}

var baseSystemMessage = `You are a very helpful assistant. Your job is to choose the best possible action to solve the user question or task. Don't make assumptions about what values to plug into functions. Ask for clarification if a user request is ambiguous.`

// HandleInvoke is the handler for POST /invoke
func (h *Handler) HandleInvoke(w http.ResponseWriter, r *http.Request) {
	var (
		ctx     = r.Context()
		transID = FromTransIDContext(ctx)
		ww      = w.(EventResponseWriter)
	)
	defer r.Body.Close()

	req, err := DecodeRequest[ai.InvokeRequest](r, w, h.service.Logger())
	if err != nil {
		ww.RecordError(errors.New("bad request"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), RequestTimeout)
	defer cancel()

	var (
		caller = FromCallerContext(ctx)
		tracer = FromTracerContext(ctx)
	)

	w.Header().Set("Content-Type", "application/json")

	res, err := h.service.GetInvoke(ctx, req.Prompt, baseSystemMessage, transID, caller, req.IncludeCallStack, tracer)
	if err != nil {
		ww.RecordError(err)
		RespondWithError(w, http.StatusInternalServerError, err, h.service.Logger())
		return
	}

	_ = json.NewEncoder(w).Encode(res)
}

// HandleChatCompletions is the handler for POST /chat/completions
func (h *Handler) HandleChatCompletions(w http.ResponseWriter, r *http.Request) {
	var (
		ctx     = r.Context()
		transID = FromTransIDContext(ctx)
		ww      = w.(EventResponseWriter)
	)
	defer r.Body.Close()

	req, err := DecodeRequest[openai.ChatCompletionRequest](r, w, h.service.Logger())
	if err != nil {
		ww.RecordError(err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), RequestTimeout)
	defer cancel()

	var (
		caller = FromCallerContext(ctx)
		tracer = FromTracerContext(ctx)
	)

	if err := h.service.GetChatCompletions(ctx, req, transID, caller, ww, tracer); err != nil {
		ww.RecordError(err)
		if err == context.Canceled {
			return
		}
		if ww.IsStream() {
			h.service.Logger().Error("bridge server error", "err", err.Error(), "err_type", reflect.TypeOf(err).String())
			w.Write([]byte(fmt.Sprintf(`{"error":{"message":"%s"}}`, err.Error())))
			return
		}
		RespondWithError(w, http.StatusBadRequest, err, h.service.Logger())
		return
	}
}

// DecodeRequest decodes the request body into given type.
func DecodeRequest[T any](r *http.Request, w http.ResponseWriter, logger *slog.Logger) (T, error) {
	var req T
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		RespondWithError(w, http.StatusBadRequest, err, logger)
		return req, err
	}

	return req, nil
}
