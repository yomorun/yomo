package server

import (
	"errors"
	"os"
	"strings"

	"gopkg.in/yaml.v2"
)

// Config represents a YoMo Zipper config.
type Config struct {
	Name      string     `yaml:"name"`
	Host      string     `yaml:"host"`
	Port      int        `yaml:"port"`
	Functions []Function `yaml:"functions"`
}

// App represents a YoMo Stream Function.
type Function struct {
	Name string `yaml:"name"`
}

// Load the Config by path.
func Load(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}

	fileSize := fileInfo.Size()
	buffer := make([]byte, fileSize)

	_, err = file.Read(buffer)
	if err != nil {
		return nil, err
	}

	return load(buffer)
}

func load(data []byte) (*Config, error) {
	var config = &Config{}
	err := yaml.Unmarshal(data, config)
	if err != nil {
		return nil, err
	}
	return config, nil
}

// ParseConfig parses the config.
func ParseConfig(config string) (*Config, error) {
	if !(strings.HasSuffix(config, ".yaml") || strings.HasSuffix(config, ".yml")) {
		return nil, errors.New(`The extension of workflow config is incorrect, it should ".yaml|.yml"`)
	}

	// parse workflow.yaml
	wfConf, err := Load(config)
	if err != nil {
		return nil, errors.New("Parse the workflow config failure with the error: " + err.Error())
	}

	// validate
	err = validateConfig(wfConf)
	if err != nil {
		return nil, err
	}

	return wfConf, nil
}

func validateConfig(wfConf *Config) error {
	if wfConf == nil {
		return errors.New("conf is nil")
	}

	m := map[string][]Function{
		"Functions": wfConf.Functions,
	}

	missingParams := []string{}
	for k, apps := range m {
		for _, app := range apps {
			if app.Name == "" {
				missingParams = append(missingParams, k)
			}
		}
	}

	errMsg := ""
	if wfConf.Name == "" || wfConf.Host == "" || wfConf.Port <= 0 {
		errMsg = "Missing name, host or port in workflow config. "
	}

	if len(missingParams) > 0 {
		errMsg += "Missing name, host or port in " + strings.Join(missingParams, ", "+". ")
	}

	if errMsg != "" {
		return errors.New(errMsg)
	}

	return nil
}
