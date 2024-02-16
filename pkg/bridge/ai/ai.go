package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"

	"gopkg.in/yaml.v3"

	gonanoid "github.com/matoous/go-nanoid/v2"
	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/serverless"
	"golang.org/x/exp/slog"
)

const (
	DefaultZipperAddr              = "localhost:9000"
	DefaultChatCompletionsEndpoint = "/chat/completions"
)

var (
	ErrNotExistsProvider     = errors.New("llm provider does not exist")
	ErrNotImplementedService = errors.New("llm service is not implemented")
	ErrConfigNotFound        = errors.New("ai config was not found")
	ErrConfigFormatError     = errors.New("ai config format is incorrect")
	ErrNotFoundEndpoint      = errors.New("endpoint was not found")
)

// AIService provides an interface to the llm api
type AIService interface {
	// GetChatCompletions returns the chat completions
	GetChatCompletions(appID string, prompt string) (*ai.ChatCompletionsResponse, error)
}

// AIProvider provides an interface to the llm providers
type AIProvider interface {
	// Name returns the name of the llm provider
	Name() string
	// RegisterFunction registers the llm function
	RegisterFunction(appID string, tag uint32, functionDefinition *ai.FunctionDefinition) error
	// UnregisterFunction unregister the llm function
	UnregisterFunction(appID string, name string) error
	// ListToolCalls lists the llm tool calls
	ListToolCalls(appID string) (map[uint32]ai.ToolCall, error)
}

// ======================= AIProvider =======================
var (
	providers       sync.Map
	defaultProvider AIProvider
)

// RegisterProvider registers the llm provider
func RegisterProvider(provider AIProvider) {
	if provider != nil {
		providers.Store(provider.Name(), provider)
	}
}

// ListProviders returns the list of llm providers
func ListProviders() []string {
	var names []string
	providers.Range(func(key, value any) bool {
		names = append(names, key.(string))
		return true
	})
	return names
}

// SetDefaultProvider sets the default llm provider
func SetDefaultProvider(name string) {
	provider := GetProvider(name)
	if provider != nil {
		defaultProvider = provider
	}
}

// GetProvider returns the llm provider by name
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

// AIServer provides restful service for user
type AIServer struct {
	// Name is the name of the AI server
	Name string
	// AIService is the llm service which provides the function calling ability
	AIService
	// Config is the configuration of the AI server
	*Config
	// ZipperAddr is the address of the zipper
	ZipperAddr string
}

// NewAIServer creates a new restful service
func NewAIServer(name string, config *Config, service AIService, zipperAddr string) (*AIServer, error) {
	zipperAddr = parseZipperAddr(zipperAddr)
	return &AIServer{
		Name:       name,
		AIService:  service,
		Config:     config,
		ZipperAddr: zipperAddr,
	}, nil
}

// Serve starts a RESTful service that provides a '/call' endpoint.
// Users submit questions to this endpoint. The service then generates a prompt based on the question and
// registered functions. It calls the LLM service from the LLM provider to get the functions and arguments to be
// invoked. These functions are invoked sequentially by YoMo. The result of the last function invocation is
// returned as the response to the user's question.
func (a *AIServer) Serve() error {
	handler := http.NewServeMux()

	// read the service endpoint from the config
	pattern := fmt.Sprintf("/%s", a.Name)
	handler.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf("[%s] AI Server is running", a.Name)))
	})
	// chat completions
	// chatCompletions := a.Config.Server.Endpoints["chat_completions"]
	pattern = fmt.Sprintf("/%s%s", a.Name, a.Config.Server.Endpoints["chat_completions"])

	// create a cache to store the http.ResponseWriter, the key is the reqID
	cache := make(map[string]http.ResponseWriter)

	// create a sfn to handle the result of the function invocation
	sfn := yomo.NewStreamFunction("fc-reducer", a.ZipperAddr, yomo.SfnOption(yomo.WithCredential("token:Happy New Year")))
	defer sfn.Close()
	sfn.SetObserveDataTags(0x61)
	sfn.SetHandler(func(ctx serverless.Context) {
		buf := ctx.Data()
		slog.Info("<<sfn", "tag", 0x61, "data", string(buf))
		reqID := string(buf[:6])
		w, ok := cache[reqID]
		if !ok {
			slog.Error("reqID not found", "reqID", reqID)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(buf[6:])
		w.Write([]byte("cccccccc\n"))
	})
	err := sfn.Connect()
	if err != nil {
		slog.Error("[sfn-reducer] connect", "err", err)
		return err
	}

	// create the handler for the endpoint
	handler.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		log := slog.With("path", pattern, "method", r.Method)
		defer r.Body.Close()

		reqID, err := gonanoid.New(6)
		if err != nil {
			log.Error("generate reqID", "err", err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		cache[reqID] = w
		log.Info("reqID", "val", reqID)

		var req ai.ChatCompletionsRequest
		// set json response
		w.Header().Set("Content-Type", "application/json")

		// decode the request
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

		// call llm to infer the function and arguments to be invoked
		slog.Info(">>>>llm request", "appID", req.AppID, "prompt", req.Prompt)
		resp, err := a.GetChatCompletions(req.AppID, req.Prompt)
		if err != nil {
			log.Error("invoke service", "err", err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		slog.Debug(">>llm response", "content", resp.Content)

		err = json.NewEncoder(w).Encode(resp)
		if err != nil {
			log.Error("encode response", "err", err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		// w.WriteHeader(http.StatusOK)

		for tag, fns := range resp.Functions {
			for _, fn := range fns {
				log := slog.With("tag", tag, "function", fn.Name, "arguments", fn.Arguments)
				log.Info("invoke func", "tag", tag, "function", fn.Name, "arguments", fn.Arguments, "reqID", reqID)
				data := SfnInvokeParameters{ReqID: reqID, Arguments: fn.Arguments}
				// err := app.Source.Write(tag, []byte(fn.Arguments))
				err := app.Source.Write(tag, data.Bytes())
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

// SfnInvokeParameters describes the data structure when invoking the sfn function
type SfnInvokeParameters struct {
	ReqID     string
	Arguments string
}

func (sip *SfnInvokeParameters) Bytes() []byte {
	buf1 := []byte(sip.ReqID)
	buf2 := []byte(sip.Arguments)
	return append(buf1, buf2...)
}

// ======================= Package Functions =======================
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
	slog.Info("register function", "appID", appID, "name", fd.Name, "tag", tag, "function", string(functionDefinition))
	return provider.RegisterFunction(appID, tag, &fd)
}

// UnregisterFunction unregister the AI function
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
	// auth := req.Header.Get("Authorization")
	// if auth == "" {
	// 	return ""
	// }
	// if !strings.HasPrefix(auth, "Bearer") {
	// 	slog.Error("invalid Authorization header", "header", auth)
	// 	return ""
	// }
	// token := strings.TrimPrefix(auth, "Bearer ")
	return "token:Happy New Year"
}
