// Package ai provide LLM Function Calling features
package ai

import (
	"encoding/json"
	"errors"
	"net"

	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/ylog"
	"github.com/yomorun/yomo/pkg/bridge/ai/register"
	"gopkg.in/yaml.v3"
)

var (
	// ErrConfigNotFound is the error when the ai config was not found
	ErrConfigNotFound = errors.New("ai config was not found")
	// ErrConfigFormatError is the error when the ai config format is incorrect
	ErrConfigFormatError = errors.New("ai config format is incorrect")
)

// RegisterFunctionMW returns a ConnMiddleware that can be used to register an ai function.
func RegisterFunctionMW() core.ConnMiddleware { return registerFunction(register.GetRegister()) }

func registerFunction(r register.Register) core.ConnMiddleware {
	return core.ConnMiddleware(func(next core.ConnHandler) core.ConnHandler {
		return func(conn *core.Connection) {
			connMd := conn.Metadata().Clone()
			definition, ok := connMd.Get(ai.FunctionDefinitionKey)

			defer func() {
				if ok {
					conn.Metadata().Set(ai.FunctionDefinitionKey, "")
				}
				// definition does not be transmitted in mesh network, It only works for handshake.
				next(conn)
				if ok {
					register.UnregisterFunction(conn.ID(), connMd)
					conn.Logger.Info("unregister ai function", "name", conn.Name(), "connID", conn.ID())
				}
			}()

			if conn.ClientType() != core.ClientTypeStreamFunction || !ok {
				return
			}

			for _, tag := range conn.ObserveDataTags() {
				// register ai function
				fd := ai.FunctionDefinition{}
				err := json.Unmarshal([]byte(definition), &fd)
				if err != nil {
					conn.Logger.Error("unmarshal function definition", "error", err)
					return
				}
				err = r.RegisterFunction(tag, &fd, conn.ID(), connMd)
				if err != nil {
					conn.Logger.Error("failed to register ai function", "name", conn.Name(), "tag", tag, "err", err)
					return
				}
				conn.Logger.Info("register ai function success", "name", conn.Name(), "tag", tag, "definition", string(definition))
			}
		}
	})
}

// Config is the configuration of AI bridge.
// The configuration looks like:
//
// bridge:
//
//	ai:
//		server:
//			host: http://localhost
//			port: 8000
//			credential: token:<CREDENTIAL>
//			provider: openai
//		providers:
//			azopenai:
//				api_endpoint: https://<RESOURCE>.openai.azure.com
//				deployment_id: <DEPLOYMENT_ID>
//				api_key: <API_KEY>
//				api_version: <API_VERSION>
//			openai:
//				api_key:
//				api_endpoint:
//			gemini:
//				api_key:
//			cloudflare_azure:
//				endpoint: https://gateway.ai.cloudflare.com/v1/<CF_GATEWAY_ID>/<CF_GATEWAY_NAME>
//				api_key: <AZURE_API_KEY>
//				resource: <AZURE_OPENAI_RESOURCE>
//				deployment_id: <AZURE_OPENAI_DEPLOYMENT_ID>
//				api_version: <AZURE_OPENAI_API_VERSION>
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
//		addr: host:port
//		provider: azopenai

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
