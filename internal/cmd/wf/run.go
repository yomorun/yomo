package wf

import (
	"log"

	"github.com/spf13/cobra"
	"github.com/yomorun/yomo/internal/workflow"
)

// RunOptions are the options for run command.
type RunOptions struct {
	baseOptions
}

// NewCmdRun creates a new command run.
func NewCmdRun() *cobra.Command {
	var opts = &RunOptions{}

	var cmd = &cobra.Command{
		Use:   "run",
		Short: "Run a YoMo Serverless Function",
		Long:  "Run a YoMo Serverless Function with mocking yomo-source data from YCloud.",
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
