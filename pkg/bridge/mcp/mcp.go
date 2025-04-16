package mcp

import (
	"errors"

	"github.com/yomorun/yomo/core/ylog"
	"gopkg.in/yaml.v3"
)

var (
	// ErrConfigNotFound is the error when the mcp config was not found
	ErrConfigNotFound = errors.New("mcp config was not found")
	// ErrConfigFormatError is the error when the ai config format is incorrect
	ErrConfigFormatError = errors.New("mcp config format is incorrect")
)

type Config struct {
	Server Server `yaml:"server"` // Server is the configuration of the mcp server
}

// Server is the configuration of the mcp server, which is the endpoint for end user access
type Server struct {
	Addr string `yaml:"addr"` // Addr is the address of the server
}

// ParseConfig parses the AI config from conf
func ParseConfig(conf map[string]any) (config *Config, err error) {
	section, ok := conf["mcp"]
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
		ylog.Error("marshal mcp config", "err", err.Error())
		return
	}
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		ylog.Error("unmarshal mcp config", "err", err.Error())
		return
	}
	// defaults values
	if config.Server.Addr == "" {
		config.Server.Addr = ":9090"
	}
	return
}
