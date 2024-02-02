package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/frame"
	"golang.org/x/exp/slog"
)

var (
	ErrNotExistsProvider     = errors.New("not exists AI provider")
	ErrNotImplementedService = errors.New("not implemented AI service")
	ErrConfigNotFound        = errors.New("ai config not found")
	ErrConfigFormatError     = errors.New("ai config format error")
)

// AIService provides an interface to the AI API
type AIService interface {
	GetChatCompletions(appID string, tag uint32, prompt string) (*ai.ChatCompletionsResponse, error)
}

// AIProvider
type AIProvider interface {
	Name() string
	RegisterFunction(appID string, tag uint32, functionDefinition []byte) error
	UnregisterFunction(appID string, tag uint32) error
	ListToolCalls(appID string, tag uint32) ([]ai.ToolCall, error)
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
	Config
	Source yomo.Source
}

func NewAIServer(name string, config Config, service AIService) (*AIServer, error) {
	source := yomo.NewSource(
		name,
		"localhost:9000",
		yomo.WithSourceReConnect(),
		yomo.WithCredential(config.Server.Credential),
	)
	// create ai source
	err := source.Connect()
	if err != nil {
		slog.Error("source connect failure", "err", err.Error())
		return nil, err
	}
	return &AIServer{
		Name:      name,
		AIService: service,
		Config:    config,
		// TODO: shuold be pools of source, for maintain multiple applications
		Source: source,
	}, nil
}

func (a *AIServer) Serve() error {
	handler := http.NewServeMux()

	pattern := fmt.Sprintf("/%s", a.Name)
	handler.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf("[%s] AI Server is running", a.Name)))
	})

	pattern = fmt.Sprintf("/%s/chat/completions", a.Name)
	handler.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		// TODO: need to returns json
		log := slog.With("path", pattern, "method", r.Method)
		var req ai.ChatCompletionsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			log.Error("decode request", "err", err.Error())
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			return
		}
		// ai function
		resp, err := a.GetChatCompletions(req.AppID, req.Tag, req.Prompt)
		if err != nil {
			log.Error("invoke service", "err", err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}
		w.WriteHeader(http.StatusOK)
		err = json.NewEncoder(w).Encode(resp)
		if err != nil {
			log.Error("encode response", "err", err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}
		w.WriteHeader(http.StatusOK)
		for _, fn := range resp.Functions {
			log := slog.With("function", fn.Name, "arguments", fn.Arguments)
			log.Info("send data to zipper")
			err := a.Source.Write(req.Tag, []byte(fn.Arguments))
			if err != nil {
				log.Error("send data to zipper", "err", err.Error())
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(err.Error()))
				return
			}
		}
	})

	httpServer := http.Server{
		Addr:    a.Config.Server.Addr,
		Handler: handler,
	}
	slog.Info("AI Server is running", "addr", httpServer.Addr, "ai_provider", a.Name)
	return httpServer.ListenAndServe()
}

// ======================= Packge Functions =======================
func Serve(conf map[string]any) error {
	config, err := parseConfig(conf)
	if err != nil {
		slog.Error("parse config", "err", err.Error())
		return err
	}
	provider := GetProvider(config.Server.Provider)
	if provider == nil {
		return ErrNotExistsProvider
	}
	// provider, err := GetDefaultProvider()
	// if err != nil {
	// 	return err
	// }
	if aiService, ok := provider.(AIService); ok {
		aiServer, err := NewAIServer(provider.Name(), config, aiService)
		if err != nil {
			return err
		}
		return aiServer.Serve()
	}
	slog.Warn("not exists AI service")
	return nil
}

func RegisterFunction(appID string, tag uint32, functionDefinition []byte) error {
	provider, err := GetDefaultProvider()
	if err != nil {
		return err
	}
	slog.Debug("register function", "appID", appID, "tag", tag, "function", string(functionDefinition))
	return provider.RegisterFunction(appID, tag, functionDefinition)
}

func UnregisterFunction(appID string, tag uint32) error {
	provider, err := GetDefaultProvider()
	if err != nil {
		return err
	}
	return provider.UnregisterFunction(appID, tag)
}

func ListToolCalls(appID string, tag uint32) ([]ai.ToolCall, error) {
	provider, err := GetDefaultProvider()
	if err != nil {
		return nil, err
	}
	return provider.ListToolCalls(appID, tag)
}

func GetChatCompletions(appID string, tag uint32, prompt string) (*ai.ChatCompletionsResponse, error) {
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

// ConnMiddleware returns a ConnMiddleware that can be used to intercept the connection.
func ConnMiddleware(next core.ConnHandler) core.ConnHandler {
	return func(conn *core.Connection) {
		// for {
		f, err := conn.FrameConn().ReadFrame()
		if err != nil {
			conn.Logger.Info("failed to read frame", "err", err)
			return
		}
		if ff, ok := f.(*frame.AIRegisterFunctionFrame); ok {
			err := conn.FrameConn().WriteFrame(&frame.AIRegisterFunctionAckFrame{AppID: ff.AppID, Tag: ff.Tag})
			if err != nil {
				conn.Logger.Error("failed to write ai RegisterFunctionAckFrame", "app_id", ff.AppID, "tag", ff.Tag, "err", err)
				return
			}
			// register ai function
			err = RegisterFunction(ff.AppID, ff.Tag, ff.Definition)
			if err != nil {
				conn.Logger.Error("failed to register ai function", "app_id", ff.AppID, "tag", ff.Tag, "err", err)
				return
			}
			conn.Logger.Info("register ai function success", "app_id", ff.AppID, "tag", ff.Tag, "definition", string(ff.Definition))
		}
		next(conn)
		// }
	}
}

// server:
//   host: http://localhost
//   port: 8000
//   endpoints:
//     chat_completions: /chat/completions
//   credential: token:<CREDENTIAL>
//   provider: azopenai
//
// providers:
//   azopenai:
//     api_key:
//     api_endpoint:
//
//   openai:
//     api_key:
//     api_endpoint:
//
//   huggingface:
//     model:

// Config is the configuration of AI bridge
type Config struct {
	Server    Server              `yaml:"server"`
	Providers map[string]Provider `yaml:"providers"`
}

// Server is the configuration of AI server
type Server struct {
	Addr       string            `yaml:"addr"`
	Endpoints  map[string]string `yaml:"endpoints"`
	Credential string            `yaml:"credential"`
	Provider   string            `yaml:"provider"`
}

// Provider is the configuration of AI provider
type Provider = map[string]string

// map[ai:
//	map[providers:
//		map[azopenai:
//			map[api_endpoint:<nil>
//					api_key:<nil>]
//				huggingface:map[model:<nil>]
//				openai:map[api_endpoint:<nil> api_key:<nil>]]
//	server:map[
//		credential:token:<CREDENTIAL>
//		endpoints:map[chat_completions:/chat/completions]
//		host:http://localhost
//		port:8000
//		provider:azopenai]]]

// parseConfig parses the config from conf
func parseConfig(conf map[string]any) (config Config, err error) {
	section, ok := conf["ai"]
	if !ok {
		err = ErrConfigNotFound
		return
	}
	aiConfig, ok := section.(map[string]any)
	if !ok {
		err = ErrConfigFormatError
		return
	}
	data, e := yaml.Marshal(aiConfig)
	if e != nil {
		err = e
		slog.Error("marshal ai config", "err", err.Error())
		return
	}
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		slog.Error("unmarshal ai config", "err", err.Error())
		return
	}
	// defaults values
	if config.Server.Addr == "" {
		config.Server.Addr = ":8000"
	}
	slog.Info("parse config", "config", config)
	return
}
