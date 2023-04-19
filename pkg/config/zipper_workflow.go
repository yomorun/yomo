package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// App represents a YoMo Application.
type App struct {
	Name string `yaml:"name"`
}

// Workflow represents a YoMo Workflow.
type Workflow struct {
	Functions []App `yaml:"functions"`
}

// WorkflowConfig represents a YoMo Workflow config.
type WorkflowConfig struct {
	// Name represents the name of the zipper.
	Name string `yaml:"name"`
	// Host represents the listening host of the zipper.
	Host string `yaml:"host"`
	// Port represents the listening port of the zipper.
	Port int `yaml:"port"`
	// Workflow represents the sfn workflow.
	Workflow `yaml:",inline"`
}

// ErrWorkflowConfigExt represents the extension of workflow config is incorrect.
var ErrWorkflowConfigExt = errors.New(`workflow: the extension of workflow config is incorrect, it should ".yaml|.yml"`)

// LoadWorkflowConfig the WorkflowConfig by path.
func LoadWorkflowConfig(path string) (*WorkflowConfig, error) {
	buffer, err := os.ReadFile(path)
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

// ParseWorkflowConfig parses the config.
func ParseWorkflowConfig(config string) (*WorkflowConfig, error) {
	if ext := filepath.Ext(config); ext != ".yaml" && ext != ".yml" {
		return nil, ErrWorkflowConfigExt
	}

	// parse workflow.yaml
	wfConf, err := LoadWorkflowConfig(config)
	if err != nil {
		return nil, err
	}

	// validate
	err = validateWorkflowConfig(wfConf)
	if err != nil {
		return nil, err
	}

	return wfConf, nil
}

func validateWorkflowConfig(wfConf *WorkflowConfig) error {
	if wfConf == nil {
		return errors.New("workflow: config nil")
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