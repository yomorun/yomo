package cmd

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"
	"github.com/yomorun/yomo/internal/serverless"
)

// DevOptions are the options for dev command.
type DevOptions struct {
	baseOptions
	// Port is the port number of UDP host for Serverless function (default is 4242).
	Port int
}

// NewCmdDev creates a new command dev.
func NewCmdDev() *cobra.Command {
	var opts = &DevOptions{}

	var cmd = &cobra.Command{
		Use:   "dev",
		Short: "Dev a YoMo Serverless Function.",
		Long:  "Dev a YoMo Serverless Function with mocking yomo-source data from YCloud.",
		Run: func(cmd *cobra.Command, args []string) {
			slHandler, err := buildAndLoadHandler(&opts.baseOptions, args)
			if err != nil {
				return
			}

			// serve the Serverless app
			endpoint := fmt.Sprintf("0.0.0.0:%d", opts.Port)
			quicHandler := &quicServerHandler{
				serverlessHandle: slHandler,
			}

			err = serverless.Run(endpoint, quicHandler)
			if err != nil {
				log.Print("Run the serverless failure with err: ", err)
			}
		},
	}

	cmd.Flags().StringVar(&opts.Filename, "file-name", "app.go", "Serverless function file (default is app.go)")
	cmd.Flags().IntVar(&opts.Port, "port", 4242, "Port is the port number of UDP host for Serverless function (default is 4242)")

	return cmd
}
