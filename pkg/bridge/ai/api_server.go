package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	openai "github.com/sashabaranov/go-openai"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider"
	"github.com/yomorun/yomo/pkg/bridge/ai/register"
	"github.com/yomorun/yomo/pkg/id"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	// DefaultZipperAddr is the default endpoint of the zipper
	DefaultZipperAddr = "localhost:9000"
	// RequestTimeout is the timeout for the request, default is 90 seconds
	RequestTimeout = 90 * time.Second
	//  RunFunctionTimeout is the timeout for awaiting the function response, default is 60 seconds
	RunFunctionTimeout = 60 * time.Second
)

// BasicAPIServer provides restful service for end user
type BasicAPIServer struct {
	zipperAddr  string
	credential  string
	httpHandler http.Handler
}

// Serve starts the Basic API Server
func Serve(config *Config, zipperListenAddr string, credential string, logger *slog.Logger) error {
	provider, err := provider.GetProvider(config.Server.Provider)
	if err != nil {
		return err
	}
	srv, err := NewBasicAPIServer(config, zipperListenAddr, credential, provider, logger)
	if err != nil {
		return err
	}

	logger.Info("start AI Bridge service", "addr", config.Server.Addr, "provider", provider.Name())
	return http.ListenAndServe(config.Server.Addr, srv.httpHandler)
}

// NewServeMux creates a new http.ServeMux for the llm bridge server.
func NewServeMux(service *Service) *http.ServeMux {
	var (
		h   = &Handler{service}
		mux = http.NewServeMux()
	)
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

// NewBasicAPIServer creates a new restful service
func NewBasicAPIServer(config *Config, zipperAddr, credential string, provider provider.LLMProvider, logger *slog.Logger) (*BasicAPIServer, error) {
	zipperAddr = parseZipperAddr(zipperAddr)

	logger = logger.With("component", "bridge")

	service := NewService(zipperAddr, provider, &ServiceOptions{
		Logger:         logger,
		Tracer:         otel.Tracer("yomo-llm-bridge"),
		CredentialFunc: func(r *http.Request) (string, error) { return credential, nil },
	})

	mux := NewServeMux(service)

	server := &BasicAPIServer{
		zipperAddr:  zipperAddr,
		credential:  credential,
		httpHandler: DecorateHandler(mux, decorateReqContext(service, logger)),
	}

	return server, nil
}

// decorateReqContext decorates the context of the request, it injects a transID into the request's context,
// log the request information and start tracing the request.
func decorateReqContext(service *Service, logger *slog.Logger) func(handler http.Handler) http.Handler {
	host, _ := os.Hostname()

	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			caller, err := service.LoadOrCreateCaller(r)
			if err != nil {
				RespondWithError(w, http.StatusBadRequest, err, logger)
				return
			}
			ctx = WithCallerContext(ctx, caller)

			// trace every request
			ctx, span := service.option.Tracer.Start(
				ctx,
				r.URL.Path,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(attribute.String("host", host)),
			)
			defer span.End()

			transID := id.New(32)
			ctx = WithTransIDContext(ctx, transID)

			logger.Info("request", "method", r.Method, "path", r.URL.Path, "transID", transID)

			handler.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// Handler handles the http request.
type Handler struct {
	service *Service
}

// HandleOverview is the handler for GET /overview
func (h *Handler) HandleOverview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	tcs, err := register.ListToolCalls(FromCallerContext(r.Context()).Metadata())
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, err, h.service.logger)
		return
	}

	functions := make(map[uint32]*openai.FunctionDefinition)
	for tag, tc := range tcs {
		functions[tag] = tc.Function
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(&ai.OverviewResponse{Functions: functions})
}

var baseSystemMessage = `You are a very helpful assistant. Your job is to choose the best possible action to solve the user question or task. Don't make assumptions about what values to plug into functions. Ask for clarification if a user request is ambiguous.`

// HandleInvoke is the handler for POST /invoke
func (h *Handler) HandleInvoke(w http.ResponseWriter, r *http.Request) {
	var (
		ctx     = r.Context()
		transID = FromTransIDContext(ctx)
	)
	defer r.Body.Close()

	req, err := DecodeRequest[ai.InvokeRequest](r, w, h.service.logger)
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), RequestTimeout)
	defer cancel()

	w.Header().Set("Content-Type", "application/json")

	res, err := h.service.GetInvoke(ctx, req.Prompt, baseSystemMessage, transID, FromCallerContext(ctx), req.IncludeCallStack)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, err, h.service.logger)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(res)
}

// HandleChatCompletions is the handler for POST /chat/completions
func (h *Handler) HandleChatCompletions(w http.ResponseWriter, r *http.Request) {
	var (
		ctx     = r.Context()
		transID = FromTransIDContext(ctx)
	)
	defer r.Body.Close()

	req, err := DecodeRequest[openai.ChatCompletionRequest](r, w, h.service.logger)
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), RequestTimeout)
	defer cancel()

	if err := h.service.GetChatCompletions(ctx, req, transID, FromCallerContext(ctx), w); err != nil {
		RespondWithError(w, http.StatusBadRequest, err, h.service.logger)
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

// RespondWithError writes an error to response according to the OpenAI API spec.
func RespondWithError(w http.ResponseWriter, code int, err error, logger *slog.Logger) {
	logger.Error("bridge server error", "error", err)

	oerr, ok := err.(*openai.APIError)
	if ok {
		if oerr.HTTPStatusCode >= 400 {
			code = http.StatusInternalServerError
			w.WriteHeader(code)
			w.Write([]byte(fmt.Sprintf(`{"error":{"code":"%d","message":"%s"}}`, code, "Internal Server Error, Please Try Again Later.")))
			return
		}
	}
	w.WriteHeader(code)
	w.Write([]byte(fmt.Sprintf(`{"error":{"code":"%d","message":"%s"}}`, code, err.Error())))
}

func getLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}
	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		ip := ipnet.IP
		if !ok || ip.IsUnspecified() || ip.To4() == nil || ip.To16() == nil {
			continue
		}
		return ip.String(), nil
	}
	return "", errors.New("not found local ip")
}

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
