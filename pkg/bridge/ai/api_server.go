package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	openai "github.com/sashabaranov/go-openai"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core/ylog"
	"github.com/yomorun/yomo/pkg/id"
)

const (
	// DefaultZipperAddr is the default endpoint of the zipper
	DefaultZipperAddr = "localhost:9000"
)

// RequestTimeout is the timeout for the request, default is 5 seconds
var RequestTimeout = 5 * time.Second

// ======================= Interface =======================

// BasicAPIServer provides restful service for end user
type BasicAPIServer struct {
	// Name is the name of the server
	Name string
	// Config is the configuration of the server
	*Config
	// ZipperAddr is the address of the zipper
	ZipperAddr string
	// Provider is the llm provider
	Provider LLMProvider
	// serviceCredential is the credential for Function Calling Service
	serviceCredential string
}

// Serve starts the Basic API Server
func Serve(config *Config, zipperListenAddr string, credential string) error {
	provider, err := GetProviderAndSetDefault(config.Server.Provider)
	if err != nil {
		return err
	}
	srv, err := NewBasicAPIServer(provider.Name(), config, zipperListenAddr, provider, credential)
	if err != nil {
		return err
	}
	return srv.Serve()
}

// NewBasicAPIServer creates a new restful service
func NewBasicAPIServer(name string, config *Config, zipperAddr string, provider LLMProvider, credential string) (*BasicAPIServer, error) {
	zipperAddr = parseZipperAddr(zipperAddr)
	return &BasicAPIServer{
		Name:              name,
		Config:            config,
		ZipperAddr:        zipperAddr,
		Provider:          provider,
		serviceCredential: credential,
	}, nil
}

// Serve starts a RESTful service that provides a '/invoke' endpoint.
// Users submit questions to this endpoint. The service then generates a prompt based on the question and
// registered functions. It calls the completion api by llm provider to get the functions and arguments to be
// invoked. These functions are invoked sequentially by YoMo. all the functions write their results to the
// reducer-sfn.
func (a *BasicAPIServer) Serve() error {
	mux := http.NewServeMux()
	// GET /overview
	mux.HandleFunc("/overview", HandleOverview)
	// POST /invoke
	mux.HandleFunc("/invoke", HandleInvoke)
	// POST /v1/chat/completions OpenAI compatible interface
	mux.HandleFunc("/v1/chat/completions", HandleChatCompletions)

	handler := WithContextService(mux, a.serviceCredential, a.ZipperAddr, a.Provider, DefaultExchangeMetadataFunc)

	addr := a.Config.Server.Addr
	ylog.Info("server is running", "addr", addr, "ai_provider", a.Name)
	return http.ListenAndServe(addr, handler)
}

// WithContextService adds the service to the request context
func WithContextService(handler http.Handler, credential string, zipperAddr string, provider LLMProvider, exFn ExchangeMetadataFunc) http.Handler {
	// create service instance when the api server starts
	service, err := LoadOrCreateService(credential, zipperAddr, provider, exFn)
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		})
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		transID := id.New(32)
		ctx := WithTransIDContext(r.Context(), transID)
		ctx = WithServiceContext(ctx, service)
		handler.ServeHTTP(w, r.WithContext(ctx))
	})
}

// HandleOverview is the handler for GET /overview
func HandleOverview(w http.ResponseWriter, r *http.Request) {
	service := FromServiceContext(r.Context())

	w.Header().Set("Content-Type", "application/json")
	// credential := getBearerToken(r)
	resp, err := service.GetOverview()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// HandleInvoke is the handler for POST /invoke
func HandleInvoke(w http.ResponseWriter, r *http.Request) {
	var (
		ctx     = r.Context()
		service = FromServiceContext(ctx)
		transID = FromTransIDContext(ctx)
	)
	defer r.Body.Close()

	var req ai.InvokeRequest

	// decode the request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ylog.Error("decode request", "err", err.Error())
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// Create a context with a timeout of 5 seconds
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// messages
	baseSystemMessage := `You are a very helpful assistant. Your job is to choose the best possible action to solve the user question or task. Don't make assumptions about what values to plug into functions. Ask for clarification if a user request is ambiguous.`

	// Make the service call in a separate goroutine, and use a channel to get the result
	resCh := make(chan *ai.InvokeResponse, 1)
	errCh := make(chan error, 1)
	go func(service *Service, req ai.InvokeRequest, baseSystemMessage string) {
		// call llm to infer the function and arguments to be invoked
		ylog.Debug(">> ai request", "transID", transID, "prompt", req.Prompt)
		res, err := service.GetInvoke(ctx, req.Prompt, baseSystemMessage, transID, req.IncludeCallStack)
		if err != nil {
			errCh <- err
		} else {
			resCh <- res
		}
	}(service, req, baseSystemMessage)

	// Use a select statement to handle the result or timeout
	select {
	case res := <-resCh:
		ylog.Debug(">> ai response response", "res", fmt.Sprintf("%+v", res))
		// write the response to the client with res
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(res)
	case err := <-errCh:
		ylog.Error("invoke service", "err", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
	case <-ctx.Done():
		// The context was cancelled, which means the service call timed out
		w.WriteHeader(http.StatusRequestTimeout)
		json.NewEncoder(w).Encode(map[string]string{"error": "request timed out"})
	}
}

// HandleChatCompletions is the handler for POST /chat/completion
func HandleChatCompletions(w http.ResponseWriter, r *http.Request) {
	var (
		ctx     = r.Context()
		service = FromServiceContext(ctx)
		transID = FromTransIDContext(ctx)
	)
	defer r.Body.Close()

	var req openai.ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ylog.Error("decode request", "err", err.Error())
		RespondWithError(w, http.StatusBadRequest, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()

	errch := make(chan error)

	go func() {
		errch <- service.GetChatCompletions(ctx, req, transID, w, false)
	}()

	select {
	case err := <-errch:
		if err != nil {
			ylog.Error("invoke chat completions", "err", err.Error())
			RespondWithError(w, http.StatusBadRequest, err)
		}
	case <-ctx.Done():
		RespondWithError(w, http.StatusRequestTimeout, errors.New("request timed out"))
	}
}

// RespondWithError writes an error to response according to the OpenAI API spec.
func RespondWithError(w http.ResponseWriter, code int, err error) {
	w.WriteHeader(code)
	w.Header().Set("Content-Type", "application/json")
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

type serviceContextKey struct{}

// WithServiceContext adds the service to the request context
func WithServiceContext(ctx context.Context, service *Service) context.Context {
	return context.WithValue(ctx, serviceContextKey{}, service)
}

// FromServiceContext returns the service from the request context
func FromServiceContext(ctx context.Context) *Service {
	service, ok := ctx.Value(serviceContextKey{}).(*Service)
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
