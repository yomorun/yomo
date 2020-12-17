package dev

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yomorun/yomo/internal/serverless"
)

// Options are the options for dev command.
type Options struct {
	// Filename is the name of Serverless function file (default is app.go).
	Filename string
	// Port is the port number of UDP host for Serverless function (default is 4242).
	Port int
}

var opts = &Options{}

var devCmd = &cobra.Command{
	Use:   "dev",
	Short: "Run a YoMo Serverless Function",
	Long:  "Run a YoMo Serverless Function in development mode",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) >= 2 && strings.HasSuffix(args[1], ".go") {
			// the second arg of `yomo dev xxx.go` is a .go file
			opts.Filename = args[1]
		}

		// build serverless file
		log.Print("Building the Serverless Function File to serverless.go...")
		file, err := serverless.BuildFuncFile(opts.Filename)
		if err != nil {
			log.Printf("Build serverless.go failure with error: %v", err)
		} else {
			log.Print("Build serverless.go successfully!")
		}

		// run serverless.so and host the endpoint
		endpoint := fmt.Sprintf("localhost:%d", opts.Port)
		err = serverless.Run(file, endpoint)
		if err != nil {
			log.Printf("Run serverless.go failure with error: %v", err)
		}
	},
}

// Execute executes the run command.
func Execute() {
	if err := devCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	devCmd.Flags().StringVar(&opts.Filename, "file-name", "app.go", "Serverless function file (default is app.go)")
	devCmd.Flags().IntVar(&opts.Port, "port", 4242, "Port is the port number of UDP host for Serverless function (default is 4242)")
}
