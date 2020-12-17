package dev

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yomorun/yomo/internal/dispatcher"
	"github.com/yomorun/yomo/internal/serverless"
	"github.com/yomorun/yomo/pkg/quic"
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

		// build the file first
		log.Print("Building the Serverless Function File...")
		file, err := serverless.Build(opts.Filename)
		if err != nil {
			log.Print("Build serverless file failure with err: ", err)
			return
		}

		// serve the Serverless app
		endpoint := fmt.Sprintf("127.0.0.1:%d", opts.Port)
		handler := &quicServerHandler{
			filePath: file,
		}

		err = serverless.Run(endpoint, handler)
		if err != nil {
			log.Print("Run the serverless failure with err: ", err)
		}
	},
}

// Execute executes the run command.
func Execute() {
	if err := devCmd.Execute(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func init() {
	devCmd.Flags().StringVar(&opts.Filename, "file-name", "app.go", "Serverless function file (default is app.go)")
	devCmd.Flags().IntVar(&opts.Port, "port", 4242, "Port is the port number of UDP host for Serverless function (default is 4242)")
}

type quicServerHandler struct {
	filePath string
}

func (s quicServerHandler) Read(st quic.Stream) error {
	stream := rx.FromReader(st)
	stream, err := dispatcher.AutoDispatcher(s.filePath, stream)
	if err != nil {
		log.Print("AutoDispatcher failure with error: ", err)
		return err
	}

	for customer := range stream.Observe() {
		if customer.Error() {
			log.Print(customer.E.Error())
			return customer.E
		}
		log.Print("cli get: ", customer.V)
	}
	return nil
}
