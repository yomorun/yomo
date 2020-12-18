package dev

import (
	"fmt"
	"log"
	"os"
	"plugin"
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
		soFile, err := serverless.Build(opts.Filename)
		if err != nil {
			log.Print("Build serverless file failure with err: ", err)
			return
		}

		// load handle
		slHandler, err := serverless.LoadHandle(soFile)
		if err != nil {
			log.Print("Load handle from .so file failure with err: ", err)
			return
		}

		// serve the Serverless app
		endpoint := fmt.Sprintf("127.0.0.1:%d", opts.Port)
		quicHandler := &quicServerHandler{
			serverlessHandle: slHandler,
		}

		err = serverless.Run(endpoint, quicHandler)
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
	serverlessHandle plugin.Symbol
}

func (s quicServerHandler) Read(st quic.Stream) error {
	stream := rx.FromReader(st)
	dispatcher.Dispatcher(s.serverlessHandle, stream)
	return nil
}
