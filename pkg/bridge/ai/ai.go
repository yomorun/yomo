package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
	"golang.org/x/exp/slog"
)

const (
	DefaultZipperAddr              = "localhost:9000"
	DefaultChatCompletionsEndpoint = "/chat/completions"
)

var (
	ErrNotExistsProvider     = errors.New("not exists AI provider")
	ErrNotImplementedService = errors.New("not implemented AI service")
	ErrConfigNotFound        = errors.New("ai config not found")
	ErrConfigFormatError     = errors.New("ai config format error")
	ErrNotFoundEndpoint      = errors.New("not found endpoint")
)

// AIService provides an interface to the AI API
type AIService interface {
	GetChatCompletions(appID string, prompt string) (*ai.ChatCompletionsResponse, error)
}

// AIProvider
type AIProvider interface {
	Name() string
	RegisterFunction(appID string, tag uint32, functionDefinition *ai.FunctionDefinition) error
	UnregisterFunction(appID string, name string) error
	ListToolCalls(appID string) (map[uint32]ai.ToolCall, error)
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
	*Config
	ZipperAddr string
}

func NewAIServer(name string, config *Config, service AIService, zipperAddr string) (*AIServer, error) {
	zipperAddr = parseZipperAddr(zipperAddr)
	return &AIServer{
		Name:       name,
		AIService:  service,
		Config:     config,
		ZipperAddr: zipperAddr,
	}, nil
}

func (a *AIServer) Serve() error {
	handler := http.NewServeMux()
	// home
	pattern := fmt.Sprintf("/%s", a.Name)
	handler.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf("[%s] AI Server is running", a.Name)))
	})
	// chat completions
	// chatCompletions := a.Config.Server.Endpoints["chat_completions"]
	pattern = fmt.Sprintf("/%s%s", a.Name, a.Config.Server.Endpoints["chat_completions"])
	handler.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		log := slog.With("path", pattern, "method", r.Method)
		defer r.Body.Close()
		var req ai.ChatCompletionsRequest
		// set json response
		w.Header().Set("Content-Type", "application/json")

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			log.Error("decode request", "err", err.Error())
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		// get bearer token from request, it's yomo credential
		credential := getBearerToken(r)
		// invoke ai function
		app, err := a.GetOrCreateApp(req.AppID, credential)
		if err != nil {
			log.Error("get or create app", "err", err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		resp, err := a.GetChatCompletions(req.AppID, req.Prompt)
		if err != nil {
			log.Error("invoke service", "err", err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		err = json.NewEncoder(w).Encode(resp)
		if err != nil {
			log.Error("encode response", "err", err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.WriteHeader(http.StatusOK)
		for tag, fns := range resp.Functions {
			for _, fn := range fns {
				log := slog.With("tag", tag, "function", fn.Name, "arguments", fn.Arguments)
				log.Info("send data to zipper")
				err := app.Source.WriteWithTarget(tag, []byte(fn.Arguments), req.PeerID)
				if err != nil {
					log.Error("send data to zipper", "err", err.Error())
					w.WriteHeader(http.StatusInternalServerError)
					json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
					return
				}
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
// Serve starts the AI server
func Serve(config *Config, zipperListenAddr string) error {
	provider := GetProvider(config.Server.Provider)
	if provider == nil {
		return ErrNotExistsProvider
	}
	// provider, err := GetDefaultProvider()
	// if err != nil {
	// 	return err
	// }
	if aiService, ok := provider.(AIService); ok {
		aiServer, err := NewAIServer(provider.Name(), config, aiService, zipperListenAddr)
		if err != nil {
			return err
		}
		return aiServer.Serve()
	}
	slog.Warn("not exists AI service")
	return nil
}

// RegisterFunction registers the AI function
func RegisterFunction(appID string, tag uint32, functionDefinition []byte) error {
	provider, err := GetDefaultProvider()
	if err != nil {
		return err
	}
	fd := ai.FunctionDefinition{}
	err = json.Unmarshal(functionDefinition, &fd)
	if err != nil {
		slog.Error("unmarshal function definition", "error", err)
		return err
	}
	slog.Debug("register function", "appID", appID, "name", fd.Name, "tag", tag, "function", string(functionDefinition))
	return provider.RegisterFunction(appID, tag, &fd)
}

// UnregisterFunction unregisters the AI function
func UnregisterFunction(appID string, name string) error {
	provider, err := GetDefaultProvider()
	if err != nil {
		return err
	}
	return provider.UnregisterFunction(appID, name)
}

// ListToolCalls lists the AI tool calls
func ListToolCalls(appID string) (map[uint32]ai.ToolCall, error) {
	provider, err := GetDefaultProvider()
	if err != nil {
		return nil, err
	}
	return provider.ListToolCalls(appID)
}

// func GetChatCompletions(appID string, prompt string) (*ai.ChatCompletionsResponse, error) {
// 	provider, err := GetDefaultProvider()
// 	if err != nil {
// 		return nil, err
// 	}
// 	service, ok := provider.(AIService)
// 	if !ok {
// 		return nil, ErrNotImplementedService
// 	}
// 	return service.GetChatCompletions(appID, prompt)
// }

// ConnMiddleware returns a ConnMiddleware that can be used to intercept the connection.
func ConnMiddleware(next core.ConnHandler) core.ConnHandler {
	return func(conn *core.Connection) {
		// check sfn type and is ai function
		if conn.ClientType() != core.ClientTypeStreamFunction {
			next(conn)
			return
		}
		for {
			f, err := conn.FrameConn().ReadFrame()
			// unregister ai function on any error
			if err != nil {
				conn.Logger.Error("failed to read frame on ai middleware", "err", err)
				conn.Logger.Info("error type", "type", fmt.Sprintf("%T", err))
				appID, _ := conn.Metadata().Get(metadata.AppIDKey)
				name := conn.Name()
				conn.Logger.Info("unregister ai function", "app_id", appID, "name", name)
				UnregisterFunction(appID, name)
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
		}
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
	Addr      string            `yaml:"addr"`
	Endpoints map[string]string `yaml:"endpoints"`
	Provider  string            `yaml:"provider"`
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

// ParseConfig parses the AI config from conf
func ParseConfig(conf map[string]any) (config *Config, err error) {
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
	// endpoints
	if config.Server.Endpoints == nil {
		config.Server.Endpoints = map[string]string{
			"chat_completions": DefaultChatCompletionsEndpoint,
		}
	}
	slog.Info("parse AI config success")
	return
}

// parseZipperAddr parses the zipper address from zipper listen address
func parseZipperAddr(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		slog.Error("invalid zipper address, return default",
			"addr", addr,
			"default", DefaultZipperAddr,
			"err", err.Error(),
		)
		return DefaultZipperAddr
	}
	if host == "localhost" {
		return addr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		slog.Error("invalid zipper address, return default",
			"addr", addr,
			"default", DefaultZipperAddr,
		)
		return DefaultZipperAddr
	}
	if !ip.IsUnspecified() {
		addr = ip.String() + ":" + port
		// slog.Info("parse zipper address", "addr", addr)
		return addr
	}
	localIP, err := getLocalIP()
	if err != nil {
		slog.Error("get local ip, return default",
			"default", DefaultZipperAddr,
			"err", err.Error(),
		)
		return DefaultZipperAddr
	}
	return localIP + ":" + port
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

// getBearerToken returns the bearer token from the request
func getBearerToken(req *http.Request) string {
	auth := req.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	if !strings.HasPrefix(auth, "Bearer") {
		slog.Error("invalid Authorization header", "header", auth)
		return ""
	}
	token := strings.TrimPrefix(auth, "Bearer ")
	return token
}
