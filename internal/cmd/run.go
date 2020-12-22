package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"
	"github.com/yomorun/yomo/internal/serverless"
)

// RunOptions are the options for run command.
type RunOptions struct {
	// Filename is the name of Serverless function file (default is app.go).
	Filename string
	// Port is the port number of UDP host for Serverless function (default is 4242).
	Port int
}

// NewCmdRun creates a new command run.
func NewCmdRun() *cobra.Command {
	var opts = &DevOptions{}

	var cmd = &cobra.Command{
		Use:   "run",
		Short: "Run a YoMo Serverless Function",
		Long:  "Run a YoMo Serverless Function.",
		Run: func(cmd *cobra.Command, args []string) {
			slHandler, err := buildAndLoadHandler(&opts.baseOptions, args)
			if err != nil {
				return
			}

			// get YoMo env
			env := os.Getenv("YOMO_ENV")
			if env != "" {
				log.Printf("Get YOMO_ENV: %s", env)
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
