package dev

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yomorun/yomo/internal/dispatcher"
	"github.com/yomorun/yomo/pkg/rx"
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

    reader := bytes.NewReader([]byte("1"))
    stream := rx.FromReader(reader)
    stream, err := dispatcher.AutoDispatcher(opts.Filename, stream)
    if err != nil {
      fmt.Println("AutoDispatcher failure with error:", err)
    }

    for customer := range stream.Observe() {
      if customer.Error() {
        fmt.Println((customer.E.Error()))
        return
      }
      fmt.Println("cli get:", customer.V)
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
