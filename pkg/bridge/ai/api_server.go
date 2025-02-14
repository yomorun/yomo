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
	"reflect"
	"time"

	openai "github.com/sashabaranov/go-openai"
	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider"
	"github.com/yomorun/yomo/pkg/bridge/ai/register"
	"github.com/yomorun/yomo/pkg/id"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

const (
	// DefaultZipperAddr is the default endpoint of the zipper
	DefaultZipperAddr = "localhost:9000"
)

var (
	// RequestTimeout is the timeout for the request, default is 90 seconds
	RequestTimeout = 90 * time.Second
	//  RunFunctionTimeout is the timeout for awaiting the function response, default is 60 seconds
	RunFunctionTimeout = 60 * time.Second
)

// BasicAPIServer provides restful service for end user
type BasicAPIServer struct {
	httpHandler http.Handler
}

// Serve starts the Basic API Server
func Serve(config *Config, logger *slog.Logger, source yomo.Source, reducer yomo.StreamFunction) error {
	provider, err := provider.GetProvider(config.Server.Provider)
	if err != nil {
		return err
	}
	srv, err := NewBasicAPIServer(config, provider, source, reducer, logger)
	if err != nil {
		return err
	}

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
func NewBasicAPIServer(config *Config, provider provider.LLMProvider, source yomo.Source, reducer yomo.StreamFunction, logger *slog.Logger) (*BasicAPIServer, error) {
	logger = logger.With("service", "llm-bridge")

	opts := &ServiceOptions{
		Logger:         logger,
		SourceBuilder:  func(_ string) yomo.Source { return source },
		ReducerBuilder: func(_ string) yomo.StreamFunction { return reducer },
	}
	service := NewService(provider, opts)

	mux := NewServeMux(service)

	server := &BasicAPIServer{
		httpHandler: DecorateHandler(mux, decorateReqContext(service, logger)),
	}

	logger.Info("start AI Bridge service", "addr", config.Server.Addr, "provider", provider.Name())
	return server, nil
}

// decorateReqContext decorates the context of the request, it injects a transID into the request's context,
// log the request information and start tracing the request.
func decorateReqContext(service *Service, logger *slog.Logger) func(handler http.Handler) http.Handler {
	hostname, _ := os.Hostname()
	tracer := otel.Tracer("yomo-llm-bridge")

	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			ctx = WithTracerContext(ctx, tracer)

			start := time.Now()

			caller, err := service.LoadOrCreateCaller(r)
			if err != nil {
				RespondWithError(w, http.StatusBadRequest, err, logger)
				return
			}
			ctx = WithCallerContext(ctx, caller)

			// trace every request
			ctx, span := tracer.Start(
				ctx,
				r.URL.Path,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(attribute.String("host", hostname)),
			)
			defer span.End()

			transID := id.New(32)
			ctx = WithTransIDContext(ctx, transID)

			ww := NewResponseWriter(w)

			handler.ServeHTTP(ww, r.WithContext(ctx))

			duration := time.Since(start)
			if !ww.TTFT.IsZero() {
				duration = ww.TTFT.Sub(start)
			}

			logContent := []any{
				"namespace", fmt.Sprintf("%s %s", r.Method, r.URL.Path),
				"stream", ww.IsStream,
				"host", hostname,
				"requestId", transID,
				"duration", duration,
			}
			if traceID := span.SpanContext().TraceID(); traceID.IsValid() {
				logContent = append(logContent, "traceId", traceID.String())
			}
			if ww.Err != nil {
				logger.Error("llm birdge request", append(logContent, "err", ww.Err)...)
			} else {
				logger.Info("llm birdge request", logContent...)
			}
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

	json.NewEncoder(w).Encode(&ai.OverviewResponse{Functions: functions})
}

var baseSystemMessage = `You are a very helpful assistant. Your job is to choose the best possible action to solve the user question or task. Don't make assumptions about what values to plug into functions. Ask for clarification if a user request is ambiguous.`

// HandleInvoke is the handler for POST /invoke
func (h *Handler) HandleInvoke(w http.ResponseWriter, r *http.Request) {
	var (
		ctx     = r.Context()
		transID = FromTransIDContext(ctx)
		ww      = w.(*ResponseWriter)
	)
	defer r.Body.Close()

	req, err := DecodeRequest[ai.InvokeRequest](r, w, h.service.logger)
	if err != nil {
		ww.Err = errors.New("bad request")
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
		ww.Err = err
		RespondWithError(w, http.StatusInternalServerError, err, h.service.logger)
		return
	}

	_ = json.NewEncoder(w).Encode(res)
}

// HandleChatCompletions is the handler for POST /chat/completions
func (h *Handler) HandleChatCompletions(w http.ResponseWriter, r *http.Request) {
	var (
		ctx     = r.Context()
		transID = FromTransIDContext(ctx)
		ww      = w.(*ResponseWriter)
	)
	defer r.Body.Close()

	req, err := DecodeRequest[openai.ChatCompletionRequest](r, w, h.service.logger)
	if err != nil {
		ww.Err = err
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), RequestTimeout)
	defer cancel()

	var (
		caller = FromCallerContext(ctx)
		tracer = FromTracerContext(ctx)
	)

	if err := h.service.GetChatCompletions(ctx, req, transID, caller, ww, tracer); err != nil {
		ww.Err = err
		if err == context.Canceled {
			return
		}
		if ww.IsStream {
			h.service.logger.Error("bridge server error", "err", err.Error(), "err_type", reflect.TypeOf(err).String())
			w.Write([]byte(fmt.Sprintf(`{"error":{"message":"%s"}}`, err.Error())))
			return
		}
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
