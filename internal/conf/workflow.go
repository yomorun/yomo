package conf

import (
	"os"

	"gopkg.in/yaml.v2"
)

type App struct {
	Name string `yaml:"name"`
}

type Workflow struct {
	Flows []App `yaml:"flows"`
	Sinks []App `yaml:"sinks"`
}

type WorkflowConfig struct {
	Name     string `yaml:"name"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Workflow `yaml:",inline"`
}

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
