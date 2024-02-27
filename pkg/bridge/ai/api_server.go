package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	gonanoid "github.com/matoous/go-nanoid/v2"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core/ylog"
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
	handler := http.NewServeMux()

	// create service instance when the api server starts
	service, err := NewService(a.serviceCredential, a.ZipperAddr, a.Provider)
	if err != nil {
		ylog.Error("Start BasicAPIServer failed", "err", err)
		return err
	}

	// GET /overview
	handler.HandleFunc("/overview", func(w http.ResponseWriter, _ *http.Request) {
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
	})

	// POST /invoke
	handler.HandleFunc("/invoke", func(w http.ResponseWriter, r *http.Request) {
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

		var req ai.InvokeRequest
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

		// call llm to infer the function and arguments to be invoked
		ylog.Debug(">> ai request", "reqID", req.ReqID, "prompt", req.Prompt)
		resp, err := service.GetChatCompletions(req.Prompt)
		if err != nil {
			ylog.Error("invoke service", "err", err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		ylog.Debug(">> ai response", "toolCalls", fmt.Sprintf("%+v", resp.ToolCalls))

		// set Event Source response
		w.Header().Set("Content-Type", "text/event-stream")
		// w.Header().Set("Transfer-Encoding", "chunked")

		for tag, tcs := range resp.ToolCalls {
			ylog.Debug("+++invoke toolCalls", "tag", tag, "len(toolCalls)", len(tcs), "reqID", reqID)
			for _, fn := range tcs {
				// log := ylog.With("tag", tag, "function", fn.Name, "arguments", fn.Arguments)
				ylog.Info("invoke func", "tag", tag, "toolCallID", fn.ID, "function", fn.Function.Name, "arguments", fn.Function.Arguments, "reqID", reqID)
				data := &ai.FunctionCall{
					ReqID:        reqID,
					ToolCallID:   fn.ID,
					Arguments:    fn.Function.Arguments,
					FunctionName: fn.Function.Name,
					IsOK:         true,
				}
				buf, err := data.Bytes()
				if err != nil {
					ylog.Error("marshal data", "err", err.Error())
					return
				}
				err = service.Write(tag, buf)
				if err != nil {
					ylog.Error("send data to zipper", "err", err.Error())
					// w.WriteHeader(http.StatusInternalServerError)
					// json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
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
