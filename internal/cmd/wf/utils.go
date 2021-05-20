package wf

import (
	"errors"
	"log"
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

	// validate
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

	if len(wfConf.Flows) == 0 && len(wfConf.Sinks) == 0 {
		return errors.New("At least one flow or sink is required")
	}

	m := map[string][]conf.App{
		"Flows": wfConf.Flows,
		"Sinks": wfConf.Sinks,
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

func printZipperConf(wfConf *conf.WorkflowConfig) {
	log.Printf("Found %d flows in zipper config", len(wfConf.Flows))
	for i, flow := range wfConf.Flows {
		log.Printf("Flow %d: %s", i+1, flow.Name)
	}

	log.Printf("Found %d sinks in zipper config", len(wfConf.Sinks))
	for i, sink := range wfConf.Sinks {
		log.Printf("Sink %d: %s", i+1, sink.Name)
	}
}
