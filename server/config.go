package server

import (
	"errors"
	"os"
	"strings"

	"gopkg.in/yaml.v2"
)

// App represents a YoMo Application.
type App struct {
	Name string `yaml:"name"`
}

// Workflow represents a YoMo Workflow.
type Workflow struct {
	Functions []App `yaml:"functions"`
}

// Workflow represents a YoMo Workflow config.
type WorkflowConfig struct {
	Name     string `yaml:"name"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Workflow `yaml:",inline"`
}

// Load the WorkflowConfig by path.
func Load(path string) (*WorkflowConfig, error) {
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

func load(data []byte) (*WorkflowConfig, error) {
	var config = &WorkflowConfig{}
	err := yaml.Unmarshal(data, config)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func ParseConfig(config string) (*WorkflowConfig, error) {
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

func validateConfig(wfConf *WorkflowConfig) error {
	if wfConf == nil {
		return errors.New("conf is nil")
	}

	if len(wfConf.Functions) == 0 {
		return errors.New("At least one function is required")
	}

	m := map[string][]App{
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
