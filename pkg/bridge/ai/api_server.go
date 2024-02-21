package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	gonanoid "github.com/matoous/go-nanoid/v2"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/ylog"
)

const (
	DefaultZipperAddr = "localhost:9000"
)

var (
	ErrNotExistsProvider     = errors.New("llm provider does not exist")
	ErrNotImplementedService = errors.New("llm service is not implemented")
	ErrConfigNotFound        = errors.New("ai config was not found")
	ErrConfigFormatError     = errors.New("ai config format is incorrect")

	// RequestTimeout is the timeout for the request, default is 5 seconds
	RequestTimeout = 5 * time.Second
)

// ======================= Interface =======================

// BasicAPIServer provides restful service for user
type BasicAPIServer struct {
	// Name is the name of the server
	Name string
	// Config is the configuration of the server
	*Config
	// ZipperAddr is the address of the zipper
	ZipperAddr string
	// Provider is the AI provider
	Provider AIProvider
}

// NewBasicAPIServer creates a new restful service
func NewBasicAPIServer(name string, config *Config, zipperAddr string, provider AIProvider) (*BasicAPIServer, error) {
	zipperAddr = parseZipperAddr(zipperAddr)
	return &BasicAPIServer{
		Name:       name,
		Config:     config,
		ZipperAddr: zipperAddr,
		Provider:   provider,
	}, nil
}

// Serve starts a RESTful service that provides a '/invoke' endpoint.
// Users submit questions to this endpoint. The service then generates a prompt based on the question and
// registered functions. It calls the LLM service from the LLM provider to get the functions and arguments to be
// invoked. These functions are invoked sequentially by YoMo. The result of the last function invocation is
// returned as the response to the user's question.
func (a *BasicAPIServer) Serve() error {
	handler := http.NewServeMux()
	// GET /overview
	handler.HandleFunc("/overview", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		credential := getBearerToken(r)
		service, err := NewService(credential, a.ZipperAddr, a.Provider)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		resp, err := service.GetOverview()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	})

	// create a cache to store the http.ResponseWriter, the key is the reqID
	// cache := make(map[string]*CacheItem)
	//
	// a.source = yomo.NewSource(
	// 	"fc-mapper",
	// 	a.ZipperAddr,
	// 	yomo.WithSourceReConnect(),
	// 	yomo.WithCredential("token:Happy New Year"),
	// )
	// // create ai source
	// err := a.source.Connect()
	// if err != nil {
	// 	return err
	// }

	// create a sfn to handle the result of the function invocation
	// sfn := yomo.NewStreamFunction("fc-reducer", a.ZipperAddr, yomo.SfnOption(yomo.WithCredential("token:Happy New Year")))
	// defer sfn.Close()
	// sfn.SetObserveDataTags(0x61)
	// sfn.SetHandler(func(ctx serverless.Context) {
	// 	buf := ctx.Data()
	// 	ylog.Debug("<<fc-reducer", "tag", 0x61, "data", string(buf))
	// 	call, err := ai.NewFunctionCallingInvoke(ctx)
	// 	if err != nil {
	// 		ylog.Error("NewFunctionCallingParameters", "err", err.Error())
	// 		return
	// 	}
	// 	reqID := call.ReqID
	// 	v, ok := cache[reqID]
	// 	if !ok {
	// 		ylog.Error("reqID not found", "reqID", reqID)
	// 		return
	// 	}
	// 	defer v.wg.Done()
	//
	// 	// err = json.NewEncoder(v.ResponseWriter).Encode(map[string]string{"result": call.Arguments})
	// 	// if err != nil {
	// 	// 	ylog.Error("encode response", "err", err.Error())
	// 	// 	v.ResponseWriter.WriteHeader(http.StatusInternalServerError)
	// 	// 	json.NewEncoder(v.ResponseWriter).Encode(map[string]string{"error": err.Error()})
	// 	// 	return
	// 	// }
	//
	// 	v.mu.Lock()
	// 	defer v.mu.Unlock()
	//
	// 	fmt.Fprintf(v.ResponseWriter, "data: %s\n\n", call.Arguments)
	// 	// flush the response
	// 	flusher, ok := v.ResponseWriter.(http.Flusher)
	// 	if ok {
	// 		flusher.Flush()
	// 	}
	// })
	//
	// err = sfn.Connect()
	// if err != nil {
	// 	ylog.Error("[sfn-reducer] connect", "err", err)
	// 	return err
	// }

	// POST /invoke
	handler.HandleFunc("/invoke", func(w http.ResponseWriter, r *http.Request) {
		// log := ylog.With("path", pattern, "method", r.Method)
		// get bearer token from request, it's yomo credential
		credential := getBearerToken(r)
		service, err := NewService(credential, a.ZipperAddr, a.Provider)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		defer r.Body.Close()
		reqID, err := gonanoid.New(6)
		if err != nil {
			ylog.Error("generate reqID", "err", err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		ci := &CacheItem{
			wg:             &sync.WaitGroup{},
			ResponseWriter: w,
		}
		if _, ok := service.cache[reqID]; !ok {
			service.cache[reqID] = ci
		}
		ylog.Info("reqID", "val", reqID)

		var req ai.ChatCompletionsRequest
		req.ReqID = reqID

		// // set json response
		// w.Header().Set("Content-Type", "application/json")

		// decode the request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			ylog.Error("decode request", "err", err.Error())
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		// // invoke ai function
		// app, err := a.GetOrCreateApp(req.AppID, credential)
		// if err != nil {
		// 	log.Error("get or create app", "err", err.Error())
		// 	w.WriteHeader(http.StatusInternalServerError)
		// 	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		// 	return
		// }

		// call llm to infer the function and arguments to be invoked
		ylog.Debug(">> ai request", "reqID", req.ReqID, "prompt", req.Prompt)
		resp, err := service.GetChatCompletions(req.Prompt)
		if err != nil {
			ylog.Error("invoke service", "err", err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		ylog.Debug(">> ai response", "content", resp.Content)

		// err = json.NewEncoder(w).Encode(resp)
		// if err != nil {
		// 	log.Error("encode response", "err", err.Error())
		// 	w.WriteHeader(http.StatusInternalServerError)
		// 	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		// 	return
		// }

		// w.WriteHeader(http.StatusOK)

		// set Event Source response
		w.Header().Set("Content-Type", "text/event-stream")
		// w.Header().Set("Transfer-Encoding", "chunked")

		for tag, fns := range resp.Functions {
			for _, fn := range fns {
				// log := ylog.With("tag", tag, "function", fn.Name, "arguments", fn.Arguments)
				ylog.Info("invoke func", "tag", tag, "function", fn.Name, "arguments", fn.Arguments, "reqID", reqID)
				data := ai.SfnInvokeParameters{ReqID: reqID, Arguments: fn.Arguments}
				// err := app.Source.Write(tag, []byte(fn.Arguments))
				err := service.Write(tag, data.Bytes())
				if err != nil {
					ylog.Error("send data to zipper", "err", err.Error())
					w.WriteHeader(http.StatusInternalServerError)
					json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
					return
				}
				// wait for this request to be done
				ci.wg.Add(1)
			}
		}

		// wait for http response generated by all sfn-s
		// or, timeout after 5 seconds
		done := make(chan struct{})
		go func() {
			ci.wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			// continue if the waitGroup is done
			ylog.Debug("all sfn-s are done", "reqID", reqID)
			delete(service.cache, reqID)
		case <-time.After(RequestTimeout):
			// handle the timeout
			w.WriteHeader(500)
			json.NewEncoder(w).Encode(map[string]string{"error": "process timeout"})
		}
	})

	httpServer := http.Server{
		Addr:    a.Config.Server.Addr,
		Handler: handler,
	}
	ylog.Info("server is running", "addr", httpServer.Addr, "ai_provider", a.Name)
	return httpServer.ListenAndServe()
}

// ======================= Package Functions =======================
// Serve starts the Basic API Server
func Serve(config *Config, zipperListenAddr string) error {
	provider, err := GetProviderAndSetDefault(config.Server.Provider)
	if err != nil {
		return err
	}
	srv, err := NewBasicAPIServer(provider.Name(), config, zipperListenAddr, provider)
	if err != nil {
		return err
	}
	return srv.Serve()
}

// RegisterFunction registers the AI function
func RegisterFunction(tag uint32, functionDefinition []byte, connID string) error {
	provider, err := GetDefaultProvider()
	if err != nil {
		return err
	}
	fd := ai.FunctionDefinition{}
	err = json.Unmarshal(functionDefinition, &fd)
	if err != nil {
		ylog.Error("unmarshal function definition", "error", err)
		return err
	}
	ylog.Info("register function", "connID", connID, "name", fd.Name, "tag", tag, "function", string(functionDefinition))
	return provider.RegisterFunction(tag, &fd, connID)
}

// UnregisterFunction unregister the AI function
func UnregisterFunction(name string, connID string) error {
	provider, err := GetDefaultProvider()
	if err != nil {
		return err
	}
	return provider.UnregisterFunction(name, connID)
}

// ListToolCalls lists the AI tool calls
func ListToolCalls() (map[uint32]ai.ToolCall, error) {
	provider, err := GetDefaultProvider()
	if err != nil {
		return nil, err
	}
	return provider.ListToolCalls()
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
				// appID, _ := conn.Metadata().Get(metadata.AppIDKey)
				name := conn.Name()
				conn.Logger.Info("unregister ai function", "name", name, "connID", conn.ID())
				UnregisterFunction(name, conn.ID())
				return
			}
			if ff, ok := f.(*frame.AIRegisterFunctionFrame); ok {
				err := conn.FrameConn().WriteFrame(&frame.AIRegisterFunctionAckFrame{AppID: ff.AppID, Tag: ff.Tag})
				if err != nil {
					conn.Logger.Error("failed to write ai RegisterFunctionAckFrame", "app_id", ff.AppID, "tag", ff.Tag, "err", err)
					return
				}
				// register ai function
				err = RegisterFunction(ff.Tag, ff.Definition, conn.ID())
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

// Server is the configuration of server
type Server struct {
	Addr string `yaml:"addr"`
	// Endpoints map[string]string `yaml:"endpoints"`
	Provider string `yaml:"provider"`
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
		ylog.Error("marshal ai config", "err", err.Error())
		return
	}
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		ylog.Error("unmarshal ai config", "err", err.Error())
		return
	}
	// defaults values
	if config.Server.Addr == "" {
		config.Server.Addr = ":8000"
	}
	// // endpoints
	// if config.Server.Endpoints == nil {
	// 	config.Server.Endpoints = map[string]string{
	// 		"chat_completions": DefaultChatCompletionsEndpoint,
	// 	}
	// }
	ylog.Info("parse AI config success")
	return
}

// parseZipperAddr parses the zipper address from zipper listen address
func parseZipperAddr(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		ylog.Error("invalid zipper address, return default",
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
		ylog.Error("invalid zipper address, return default",
			"addr", addr,
			"default", DefaultZipperAddr,
		)
		return DefaultZipperAddr
	}
	if !ip.IsUnspecified() {
		addr = ip.String() + ":" + port
		// ylog.Info("parse zipper address", "addr", addr)
		return addr
	}
	localIP, err := getLocalIP()
	if err != nil {
		ylog.Error("get local ip, return default",
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
		ylog.Error("invalid Authorization header", "header", auth)
		return ""
	}
	token := strings.TrimPrefix(auth, "Bearer ")
	return token
	// TEST: only test
	// return "token:Happy New Year"
}
