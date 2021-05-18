package cmd

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yomorun/yomo/pkg/client"
	"github.com/yomorun/yomo/pkg/rx"
)

// RunOptions are the options for run command.
type RunOptions struct {
	baseOptions
	// Port is the port number of UDP host for Serverless function (default is 4242).
	Url  string
	Name string
}

// NewCmdRun creates a new command run.
func NewCmdRun() *cobra.Command {
	var opts = &RunOptions{}

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
			if opts.Url == "" {
				opts.Url = "localhost:9000"
			}

			splits := strings.Split(opts.Url, ":")
			if len(splits) != 2 {
				log.Printf(`❌ The format of url "%s" is incorrect, it should be "host:port", f.e. localhost:9000`, opts.Url)
				return
			}
			host := splits[0]
			port, _ := strconv.Atoi(splits[1])
			cli, err := client.NewServerless(opts.Name).Connect(host, port)
			if err != nil {
				log.Print("❌ Connect to zipper failure: ", err)
				return
			}

			hanlder := slHandler.(func(rxStream rx.RxStream) rx.RxStream)
			log.Print("Running the Serverless Function.")
			cli.Pipe(hanlder)
		},
	}

	cmd.Flags().StringVarP(&opts.Filename, "file-name", "f", "app.go", "Serverless function file")
	cmd.Flags().StringVarP(&opts.Url, "url", "u", "localhost:9000", "zipper server endpoint addr")
	cmd.Flags().StringVarP(&opts.Name, "name", "n", "", "yomo serverless app name (required). It should match the specific service name in zipper config (workflow.yaml)")
	cmd.MarkFlagRequired("name")

	return cmd
}
