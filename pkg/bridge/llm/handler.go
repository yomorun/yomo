package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	openai "github.com/sashabaranov/go-openai"
	"github.com/yomorun/yomo/ai"
	pkgai "github.com/yomorun/yomo/pkg/bridge/ai"
)

// RespondWithError writes an error to response according to the OpenAI API spec.
func RespondWithError(w pkgai.EventResponseWriter, code int, err error) error {
	newCode, errBody := w.InterceptError(code, err)

	w.RecordError(errors.New(errBody.Message))

	if newCode != 0 {
		code = newCode
	}
	w.WriteHeader(code)
	return json.NewEncoder(w).Encode(&pkgai.ErrorResponse{Error: errBody})
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
	service *pkgai.Service
}

// NewHandler return a hander that handles chat completions requests.
func NewHandler(service *pkgai.Service) *Handler {
	return &Handler{service}
}

// HandleOverview is the handler for GET /overview
func (h *Handler) HandleOverview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	ww := w.(pkgai.EventResponseWriter)

	tools, err := ai.ListToolCalls(pkgai.FromCallerContext(r.Context()).Metadata())
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
		transID = pkgai.FromTransIDContext(ctx)
		ww      = w.(pkgai.EventResponseWriter)
	)
	defer r.Body.Close()

	var req ai.InvokeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondWithError(ww, http.StatusBadRequest, err)
		ww.RecordError(errors.New("bad request"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), pkgai.RequestTimeout)
	defer cancel()

	var (
		caller = pkgai.FromCallerContext(ctx)
		tracer = pkgai.FromTracerContext(ctx)
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
		transID = pkgai.FromTransIDContext(ctx)
		ww      = w.(pkgai.EventResponseWriter)
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

	ctx, cancel := context.WithTimeout(r.Context(), pkgai.RequestTimeout)
	defer cancel()

	var (
		caller = pkgai.FromCallerContext(ctx)
		tracer = pkgai.FromTracerContext(ctx)
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
