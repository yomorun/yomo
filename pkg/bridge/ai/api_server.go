package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	openai "github.com/sashabaranov/go-openai"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider"
	"github.com/yomorun/yomo/pkg/bridge/ai/register"
	"github.com/yomorun/yomo/pkg/id"
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
	zipperAddr  string
	credential  string
	httpHandler http.Handler
	logger      *slog.Logger
}

// Serve starts the Basic API Server
func Serve(config *Config, zipperListenAddr string, credential string, logger *slog.Logger) error {
	provider, err := provider.GetProviderAndSetDefault(config.Server.Provider)
	if err != nil {
		return err
	}
	srv, err := NewBasicAPIServer(config, zipperListenAddr, provider, credential, logger)
	if err != nil {
		return err
	}

	logger.Info("start bridge server", "addr", config.Server.Addr, "provider", provider.Name())
	return srv.ServeAddr(config.Server.Addr)
}

func bridgeHTTPHanlder(decorater func(http.Handler) http.Handler) http.Handler {
	mux := http.NewServeMux()
	// GET /overview
	mux.HandleFunc("/overview", HandleOverview)
	// POST /invoke
	mux.HandleFunc("/invoke", HandleInvoke)
	// POST /v1/chat/completions (OpenAI compatible interface)
	mux.HandleFunc("/v1/chat/completions", HandleChatCompletions)

	return decorater(mux)
}

// NewBasicAPIServer creates a new restful service
func NewBasicAPIServer(config *Config, zipperAddr string, provider provider.LLMProvider, credential string, logger *slog.Logger) (*BasicAPIServer, error) {
	zipperAddr = parseZipperAddr(zipperAddr)

	cp := NewCallerProvider(zipperAddr, provider, DefaultExchangeMetadataFunc)

	server := &BasicAPIServer{
		zipperAddr:  zipperAddr,
		credential:  credential,
		httpHandler: bridgeHTTPHanlder(decorateReqContext(cp, logger, credential)),
		logger:      logger.With("component", "bridge"),
	}

	return server, nil
}

// ServeAddr starts a http server that provides some endpoints to bridge up the http server and YoMo.
// User can chat to the http server and interact with the YoMo's stream function.
func (a *BasicAPIServer) ServeAddr(addr string) error {
	return http.ListenAndServe(addr, a.httpHandler)
}

// decorateReqContext decorates the context of the request, it injects a transID and a caller into the context.
func decorateReqContext(cp *CallerProvider, logger *slog.Logger, credential string) func(handler http.Handler) http.Handler {
	return func(handler http.Handler) http.Handler {
		caller, err := cp.Provide(credential)
		if err != nil {
			logger.Info("can't load caller", "err", err)

			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				RespondWithError(w, http.StatusInternalServerError, err)
			})
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			transID := id.New(32)
			ctx := WithTransIDContext(r.Context(), transID)
			ctx = WithCallerContext(ctx, caller)

			logger.Info("request", "method", r.Method, "path", r.URL.Path, "transID", transID)

			handler.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// HandleOverview is the handler for GET /overview
func HandleOverview(w http.ResponseWriter, r *http.Request) {
	caller := FromCallerContext(r.Context())

	w.Header().Set("Content-Type", "application/json")

	tcs, err := register.ListToolCalls(caller.Metadata())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
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
func HandleInvoke(w http.ResponseWriter, r *http.Request) {
	var (
		ctx     = r.Context()
		caller  = FromCallerContext(ctx)
		transID = FromTransIDContext(ctx)
	)
	defer r.Body.Close()

	req, err := DecodeRequest[ai.InvokeRequest](r, w)
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), RequestTimeout)
	defer cancel()

	res, err := caller.GetInvoke(ctx, req.Prompt, baseSystemMessage, transID, req.IncludeCallStack)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(res)
}

// HandleChatCompletions is the handler for POST /chat/completion
func HandleChatCompletions(w http.ResponseWriter, r *http.Request) {
	var (
		ctx     = r.Context()
		caller  = FromCallerContext(ctx)
		transID = FromTransIDContext(ctx)
	)
	defer r.Body.Close()

	req, err := DecodeRequest[openai.ChatCompletionRequest](r, w)
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), RequestTimeout)
	defer cancel()

	if err := caller.GetChatCompletions(ctx, req, transID, w); err != nil {
		RespondWithError(w, http.StatusBadRequest, err)
		return
	}
}

// DecodeRequest decodes the request body into given type.
func DecodeRequest[T any](r *http.Request, w http.ResponseWriter) (T, error) {
	var req T
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		RespondWithError(w, http.StatusBadRequest, err)
		return req, err
	}

	return req, nil
}

// RespondWithError writes an error to response according to the OpenAI API spec.
func RespondWithError(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
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
	service, ok := ctx.Value(callerContextKey{}).(*Caller)
	if !ok {
		return nil
	}
	return service
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
