// Package ai provide LLM Function Calling features
package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"

	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/ylog"
	"gopkg.in/yaml.v3"
)

var (
	// ErrNotExistsProvider is the error when the provider does not exist
	ErrNotExistsProvider = errors.New("llm provider does not exist")
	// ErrNotImplementedService is the error when the service is not implemented
	ErrNotImplementedService = errors.New("llm service is not implemented")
	// ErrConfigNotFound is the error when the ai config was not found
	ErrConfigNotFound = errors.New("ai config was not found")
	// ErrConfigFormatError is the error when the ai config format is incorrect
	ErrConfigFormatError = errors.New("ai config format is incorrect")
)

// ======================= Package Functions =======================

// RegisterFunction registers the tool function
func RegisterFunction(tag uint32, functionDefinition []byte, connID uint64) error {
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
	return provider.RegisterFunction(tag, &fd, connID)
}

// UnregisterFunction unregister the tool function
func UnregisterFunction(name string, connID uint64) error {
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
				name := conn.Name()
				conn.Logger.Info("unregister ai function", "name", name, "connID", conn.ID())
				UnregisterFunction(name, conn.ID())
				return
			}
			if ff, ok := f.(*frame.AIRegisterFunctionFrame); ok {
				err := conn.FrameConn().WriteFrame(&frame.AIRegisterFunctionAckFrame{Name: ff.Name, Tag: ff.Tag})
				if err != nil {
					conn.Logger.Error("failed to write ai RegisterFunctionAckFrame", "name", ff.Name, "tag", ff.Tag, "err", err)
					return
				}
				// register ai function
				err = RegisterFunction(ff.Tag, ff.Definition, conn.ID())
				if err != nil {
					conn.Logger.Error("failed to register ai function", "name", ff.Name, "tag", ff.Tag, "err", err)
					return
				}
				conn.Logger.Info("register ai function success", "name", ff.Name, "tag", ff.Tag, "definition", string(ff.Definition))
			}
			next(conn)
		}
	}
}

// Config is the configuration of AI bridge.
// The configuration looks like:
// bridge:
//
//	ai:
//		server:
//			host: http://localhost
//			port: 8000
//			credential: token:<CREDENTIAL>
//			provider: azopenai
//
//		providers:
//			azopenai:
//				api_key:
//				api_endpoint:
//
//			openai:
//				api_key:
//				api_endpoint:
//
//			huggingface:
//				model:
type Config struct {
	Server    Server              `yaml:"server"`    // Server is the configuration of the BasicAPIServer
	Providers map[string]Provider `yaml:"providers"` // Providers is the configuration of llm provider
}

// Server is the configuration of the BasicAPIServer, which is the endpoint for end user access
type Server struct {
	Addr     string `yaml:"addr"`     // Addr is the address of the server
	Provider string `yaml:"provider"` // Provider is the llm provider to use
}

// Provider is the configuration of llm provider
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
