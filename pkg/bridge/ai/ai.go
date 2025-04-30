// Package ai provide LLM Function Calling features
package ai

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/yomorun/yomo/core/ylog"
	providerpkg "github.com/yomorun/yomo/pkg/bridge/ai/provider"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider/anthropic"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider/azopenai"
	azaifoundry "github.com/yomorun/yomo/pkg/bridge/ai/provider/azure-ai-foundry"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider/cerebras"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider/cfazure"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider/cfopenai"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider/deepseek"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider/gemini"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider/githubmodels"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider/ollama"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider/openai"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider/vertexai"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider/vllm"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider/xai"
	"gopkg.in/yaml.v3"
)

const (
	// DefaultZipperAddr is the default endpoint of the zipper
	DefaultZipperAddr = "localhost:9000"
)

var (
	// ErrConfigNotFound is the error when the ai config was not found
	ErrConfigNotFound = errors.New("ai config was not found")
	// ErrConfigFormatError is the error when the ai config format is incorrect
	ErrConfigFormatError = errors.New("ai config format is incorrect")

	RequestTimeout = 360 * time.Second
	//  RunFunctionTimeout is the timeout for awaiting the function response, default is 60 seconds
	RunFunctionTimeout = 60 * time.Second
)

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

// ParseZipperAddr parses the zipper address from zipper listen address
func ParseZipperAddr(addr string) string {
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

// NewProviderFromConfig create a llm provider from config that read from config.yaml
func NewProviderFromConfig(name string, provider map[string]string) (providerpkg.LLMProvider, error) {
	switch name {
	case "azopenai":
		return azopenai.NewProvider(
			provider["api_key"],
			provider["api_endpoint"],
			provider["deployment_id"],
			provider["api_version"],
		), nil
	case "openai":
		return openai.NewProvider(provider["api_key"], provider["model"]), nil
	case "cloudflare_azure":
		return cfazure.NewProvider(
			provider["endpoint"],
			provider["api_key"],
			provider["resource"],
			provider["deployment_id"],
			provider["api_version"],
		), nil
	case "cloudflare_openai":
		return cfopenai.NewProvider(
			provider["endpoint"],
			provider["api_key"],
			provider["model"],
		), nil
	case "ollama":
		return ollama.NewProvider(provider["api_endpoint"], provider["model"]), nil
	case "gemini":
		return gemini.NewProvider(provider["api_key"]), nil
	case "githubmodels":
		return githubmodels.NewProvider(provider["api_key"], provider["model"]), nil
	case "cerebras":
		return cerebras.NewProvider(provider["api_key"], provider["model"]), nil
	case "anthropic":
		return anthropic.NewProvider(provider["api_key"], provider["model"]), nil
	case "xai":
		return xai.NewProvider(provider["api_key"], provider["model"]), nil
	case "vertexai":
		return vertexai.NewProvider(
			provider["project_id"],
			provider["location"],
			provider["model"],
			provider["credentials_file"],
		), nil
	case "deepseek":
		return deepseek.NewProvider(provider["api_key"], provider["model"]), nil
	case "vllm":
		return vllm.NewProvider(provider["api_endpoint"], provider["api_key"], provider["model"]), nil
	case "azaifoundry":
		return azaifoundry.NewProvider(
			provider["api_endpoint"],
			provider["api_key"],
			provider["api_version"],
			provider["model"],
		), nil
	default:
		return nil, fmt.Errorf("unknown provider: %s", name)
	}
}
