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

// Auth is the way for the source or SFN to be authenticated by the zipper.
type Auth struct {
	// Type is the type of auth.
	Type string `yaml:"type"`
	// Token is the token of auth,
	// if type is token, Token is the token be authenticated.
	Token string `yaml:"token"`
}

// Config represents a yomo config.
type Config struct {
	// Name represents the name of the zipper.
	Name string `yaml:"name"`
	// Host represents the listening host of the zipper.
	Host string `yaml:"host"`
	// Port represents the listening port of the zipper.
	Port int `yaml:"port"`
	// Auth holds the information of the authectication.
	Auth Auth `yaml:"auth"`
	// Functions represents the apps that are supported in the yomo system.
	Functions []Function `yaml:"functions"`
	// Downstreams holds cascading zippers config.
	Downstreams []Downstream `yaml:"downstreams"`
}

// Downstream describes a cascading zipper config.
type Downstream struct {
	// Name is the name of downstream zipper.
	Name string `yaml:"name"`
	// Host is the host of downstream zipper.
	Host string `yaml:"host"`
	// Port is the port of downstream zipper.
	Port int `yaml:"port"`
	// Credential is the credential that specifies how downstream will authenticate the current Zipper.
	// It is in the format of 'authType:authPayload', separated by a colon.
	// If Credential is empty, it represents that downstream will not authenticate the current Zipper.
	Credential string `yaml:"credential"`
}

// ErrConfigExt represents the extension of config file is incorrect.
var ErrConfigExt = errors.New(`yomo: the extension of config is incorrect, it should ".yaml|.yml"`)

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
			return errors.New("config: the functions must have name value")
		}
	}

	return nil
}
