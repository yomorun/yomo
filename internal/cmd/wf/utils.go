package wf

import (
	"errors"
	"strings"

	"github.com/yomorun/yomo/internal/conf"
)

type baseOptions struct {
	// Config is the name of workflow config file (default is workflow.yaml).
	Config string
}

func parseConfig(opts *baseOptions, args []string) (*conf.WorkflowConfig, error) {
	if len(args) >= 1 && strings.HasSuffix(args[0], ".yaml") {
		// the second arg of `yomo wf dev xxx.yaml` is a .yaml file.
		opts.Config = args[0]
	}

	// validate opts.Config
	if opts.Config == "" {
		return nil, errors.New("Please input the file name of workflow config")
	}

	if !strings.HasSuffix(opts.Config, ".yaml") {
		return nil, errors.New(`The extension of workflow config is incorrect, it should ".yaml"`)
	}

	// parse workflow.yaml
	wfConf, err := conf.Load(opts.Config)
	if err != nil {
		return nil, errors.New("Parse the workflow config failure with the error: " + err.Error())
	}

	err = validateConfig(wfConf)
	if err != nil {
		return nil, err
	}

	return wfConf, nil
}

func validateConfig(wfConf *conf.WorkflowConfig) error {
	if wfConf == nil {
		return errors.New("conf is nil")
	}

	errMsg := ""
	if wfConf.Name == "" || wfConf.Host == "" || wfConf.Port <= 0 {
		errMsg = "Missing name, host or port in workflow config. "
	}

	if errMsg != "" {
		return errors.New(errMsg)
	}

	return nil
}
