package wf

import (
	"log"

	"github.com/spf13/cobra"
	"github.com/yomorun/yomo/internal/workflow"
)

// DevOptions are the options for dev command.
type DevOptions struct {
	baseOptions
}

// NewCmdDev creates a new command dev.
func NewCmdDev() *cobra.Command {
	var opts = &DevOptions{}

	var cmd = &cobra.Command{
		Use:   "dev",
		Short: "Dev a YoMo Serverless Function",
		Long:  "Dev a YoMo Serverless Function with mocking yomo-source data from YCloud.",
		Run: func(cmd *cobra.Command, args []string) {
			conf, err := parseConfig(&opts.baseOptions, args)
			if err != nil {
				log.Print("❌ ", err)
				return
			}

			log.Print("Running YoMo workflow...")
			err = workflow.Run(conf)
			if err != nil {
				log.Print("❌ ", err)
				return
			}
		},
	}

	cmd.Flags().StringVarP(&opts.Config, "config", "c", "workflow.yaml", "Workflow config file (default is workflow.yaml)")

	return cmd
}
