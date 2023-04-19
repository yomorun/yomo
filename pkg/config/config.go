// Package config provides configurations for cascading zippers.
package config

import (
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Function represents a yomo stream function.
type Function struct {
	// Name is the name of StreamFunction.
	Name string `yaml:"name"`
}

// Config represents a yomo config.
type Config struct {
	// Name represents the name of the zipper.
	Name string `yaml:"name"`
	// Host represents the listening host of the zipper.
	Host string `yaml:"host"`
	// Port represents the listening port of the zipper.
	Port int `yaml:"port"`
	// Auth is the way for the source or SFN to be authenticated by the zipper.
	// The token typed auth has two key-value pairs associated with it:
	// a `type:token` key-value pair and a `token:<CREDENTIAL>` key-value pair.
	Auth map[string]string `yaml:"auth"`
	// Functions represents the stream functions that zipper allowed to connect.
	Functions []Function `yaml:"functions"`
	// Downstreams holds cascading zippers config. the map-key is downstream name.
	Downstreams map[string]Downstream `yaml:"downstreams"`
}

// Downstream describes a cascading zipper config.
type Downstream struct {
	// Host is the host of downstream zipper.
	Host string `yaml:"host"`
	// Port is the port of downstream zipper.
	Port int `yaml:"port"`
	// Credential is the credential when connect to downstream zipper.
	// It is in the format of 'authType:authPayload', separated by a colon.
	// If Credential is empty, it represents that downstream will not authenticate the current Zipper.
	Credential string `yaml:"credential"`
}

// ErrConfigExt represents the extension of config file is incorrect.
var ErrConfigExt = errors.New(`yomo: the extension of config is incorrect, it should be ".yaml|.yml"`)

// ParseConfigFile parses the config from configPath. The zipper will bootstrap from this config.
func ParseConfigFile(configPath string) (Config, error) {
	if ext := filepath.Ext(configPath); ext != ".yaml" && ext != ".yml" {
		return Config{}, ErrConfigExt
	}

	buf, err := os.ReadFile(configPath)
	if err != nil {
		return Config{}, err
	}

	var config Config
	if err := yaml.Unmarshal(buf, &config); err != nil {
		return config, err
	}

	if err := validateConfig(&config); err != nil {
		return config, err
	}

	return config, nil
}

func validateConfig(conf *Config) error {
	if conf.Name == "" {
		return errors.New("config: the name is required")
	}
	if conf.Host == "" {
		return errors.New("config: the host is required")
	}
	if conf.Port == 0 {
		return errors.New("config: the port is required")
	}
	if len(conf.Functions) == 0 {
		return errors.New("config: the functions cannot be an empty")
	}

	for _, f := range conf.Functions {
		if f.Name == "" {
			return errors.New("config: the functions must have the name field")
		}
	}

	return nil
}
