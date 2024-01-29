package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"golang.org/x/exp/slog"
)

var (
	ErrNotExistsProvider     = errors.New("not exists AI provider")
	ErrNotImplementedService = errors.New("not implemented AI service")
)

// AIService provides an interface to the AI API
type AIService interface {
	GetChatCompletions(appID string, tag uint32, prompt string) (*ChatCompletionsResponse, error)
}

// AIProvider
type AIProvider interface {
	Name() string
	RegisterFunction(appID string, tag uint32, functionDefinition string) error
	UnregisterFunction(appID string, tag uint32) error
	ListToolCalls(appID string, tag uint32) ([]ToolCall, error)
}

// ======================= AIProvider =======================
var (
	providers       sync.Map
	defaultProvider AIProvider
)

func RegisterProvider(provider AIProvider) {
	if provider != nil {
		providers.Store(provider.Name(), provider)
	}
}

func ListProviders() []string {
	var names []string
	providers.Range(func(key, value any) bool {
		names = append(names, key.(string))
		return true
	})
	return names
}

func SetDefaultProvider(name string) {
	provider := GetProvider(name)
	if provider != nil {
		defaultProvider = provider
	}
}

func GetProvider(name string) AIProvider {
	if provider, ok := providers.Load(name); ok {
		return provider.(AIProvider)
	}
	return nil
}

// GetDefaultProvider returns the default AI provider
func GetDefaultProvider() (AIProvider, error) {
	if defaultProvider != nil {
		return defaultProvider, nil
	}
	names := ListProviders()
	if len(names) > 0 {
		p := GetProvider(names[0])
		if p != nil {
			return p, nil
		}
	}
	return nil, ErrNotExistsProvider
}

// ======================= AIServer =======================

type AIServer struct {
	Name string
	AIService
}

func NewAIServer(name string, service AIService) *AIServer {
	return &AIServer{
		Name:      name,
		AIService: service,
	}
}

func (a *AIServer) Serve() error {
	// TODO: need to connect to zipper with credentials
	// returns error if not connected
	handler := http.NewServeMux()

	pattern := fmt.Sprintf("/%s", a.Name)
	handler.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf("[%s] AI Server is running", a.Name)))
	})

	pattern = fmt.Sprintf("/%s/chat/completions", a.Name)

	handler.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		// TODO: need to returns json
		var req ChatCompletionsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			return
		}
		// ai function
		resp, err := a.GetChatCompletions(req.AppID, req.Tag, req.Prompt)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}
		w.WriteHeader(http.StatusOK)
		err = json.NewEncoder(w).Encode(resp)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}
		w.WriteHeader(http.StatusOK)
		// TODO: need to invoke sfn with credentials
		// for _, fn := range resp.Functions {
		// 	// TODO: invoke source send data to zipper
		// 	source.Write(req.Tag, []byte(fn.Arguments))
		// }
	})

	httpServer := http.Server{
		Addr:    ":8000", // TODO: read from config
		Handler: handler,
	}
	return httpServer.ListenAndServe()
}

// ======================= Packge Functions =======================
func Serve() error {
	// TODO: maybe multiple providers, now only one
	provider, err := GetDefaultProvider()
	if err != nil {
		return err
	}
	if aiService, ok := provider.(AIService); ok {
		aiServer := NewAIServer(provider.Name(), aiService)
		return aiServer.Serve()
	}
	slog.Warn("not exists AI service")
	return nil
}

func RegisterFunction(appID string, tag uint32, functionDefinition string) error {
	provider, err := GetDefaultProvider()
	if err != nil {
		return err
	}
	return provider.RegisterFunction(appID, tag, functionDefinition)
}

func UnregisterFunction(appID string, tag uint32) error {
	provider, err := GetDefaultProvider()
	if err != nil {
		return err
	}
	return provider.UnregisterFunction(appID, tag)
}

func ListToolCalls(appID string, tag uint32) ([]ToolCall, error) {
	provider, err := GetDefaultProvider()
	if err != nil {
		return nil, err
	}
	return provider.ListToolCalls(appID, tag)
}

func GetChatCompletions(appID string, tag uint32, prompt string) (*ChatCompletionsResponse, error) {
	provider, err := GetDefaultProvider()
	if err != nil {
		return nil, err
	}
	service, ok := provider.(AIService)
	if !ok {
		return nil, ErrNotImplementedService
	}
	return service.GetChatCompletions(appID, tag, prompt)
}
