package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// FromTracerContext returns the tracer from the request context
func FromTracerContext(ctx context.Context) trace.Tracer {
	val, ok := ctx.Value(tracerContextKey{}).(trace.Tracer)
	if !ok {
		return new(noop.Tracer)
	}
	return val
}

// RespondWithError writes an error to response according to the OpenAI API spec.
func RespondWithError(w EventResponseWriter, code int, err error) error {
	newCode, errBody := w.InterceptError(code, err)

	if newCode != 0 {
		code = newCode
	}
	w.WriteHeader(code)
	return json.NewEncoder(w).Encode(&ErrorResponse{Error: errBody})
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
	ww := w.(EventResponseWriter)

	tools, err := ai.ListToolCalls(FromCallerContext(r.Context()).Metadata())
	if err != nil {
		RespondWithError(ww, http.StatusInternalServerError, err)
		return
	}

	functions := make([]*openai.FunctionDefinition, len(tools))
	for i, tc := range tools {
		functions[i] = tc.Function
	}

	json.NewEncoder(w).Encode(&ai.OverviewResponse{Functions: functions})
}

// HandleInvoke is the handler for POST /invoke
func (h *Handler) HandleInvoke(w http.ResponseWriter, r *http.Request) {
	var (
		ctx     = r.Context()
		transID = FromTransIDContext(ctx)
		ww      = w.(EventResponseWriter)
	)
	defer r.Body.Close()

	var req ai.InvokeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondWithError(ww, http.StatusBadRequest, err)
		ww.RecordError(errors.New("bad request"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), RequestTimeout)
	defer cancel()

	var (
		caller = FromCallerContext(ctx)
		tracer = FromTracerContext(ctx)
	)
	agentContextJSON, err := json.Marshal(req.AgentContext)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		RespondWithError(ww, http.StatusBadRequest, ErrAgentContextType)
		return
	}

	if err := h.service.GetInvoke(ctx, req.Prompt, transID, caller, req.IncludeCallStack, agentContextJSON, ww, tracer); err != nil {
		RespondWithError(ww, http.StatusInternalServerError, err)
		return
	}
}

// HandleChatCompletions is the handler for POST /chat/completions
func (h *Handler) HandleChatCompletions(w http.ResponseWriter, r *http.Request) {
	var (
		ctx     = r.Context()
		transID = FromTransIDContext(ctx)
		ww      = w.(EventResponseWriter)
	)
	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		RespondWithError(ww, http.StatusBadRequest, err)
		return
	}
	req, agentContext, err := ai.DecodeChatCompletionRequest(body)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		RespondWithError(ww, http.StatusBadRequest, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), RequestTimeout)
	defer cancel()

	var (
		caller = FromCallerContext(ctx)
		tracer = FromTracerContext(ctx)
	)
	agentContextJSON, err := json.Marshal(agentContext)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		RespondWithError(ww, http.StatusBadRequest, ErrAgentContextType)
		return
	}

	if err := h.service.GetChatCompletions(ctx, req, transID, agentContextJSON, caller, ww, tracer); err != nil {
		if err == context.Canceled {
			return
		}
		if !ww.IsStream() {
			w.Header().Set("Content-Type", "application/json")
		}
		RespondWithError(ww, http.StatusBadRequest, err)
		return
	}
}

var ErrAgentContextType = errors.New("agent_context must be JSON-marshalable")
