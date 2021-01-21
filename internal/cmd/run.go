package cmd

import (
	"fmt"
	"log"
	"os"
	"plugin"

	"github.com/spf13/cobra"
	y3 "github.com/yomorun/y3-codec-golang"
	"github.com/yomorun/yomo/internal/dispatcher"
	"github.com/yomorun/yomo/internal/serverless"
	"github.com/yomorun/yomo/pkg/quic"
	"github.com/yomorun/yomo/pkg/rx"
)

// RunOptions are the options for run command.
type RunOptions struct {
	baseOptions
	// Port is the port number of UDP host for Serverless function (default is 4242).
	Port int

	mockSink bool
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

			// serve the Serverless app
			endpoint := fmt.Sprintf("0.0.0.0:%d", opts.Port)
			quicHandler := &quicServerHandler{
				serverlessHandle: slHandler,
				mockSink:         opts.mockSink,
			}

			err = serverless.Run(endpoint, quicHandler)
			if err != nil {
				log.Print("Run the serverless failure with err: ", err)
			}
		},
	}

	cmd.Flags().StringVarP(&opts.Filename, "file-name", "f", "app.go", "Serverless function file (default is app.go)")
	cmd.Flags().IntVarP(&opts.Port, "port", "p", 4242, "Port is the port number of UDP host for Serverless function (default is 4242)")
	cmd.Flags().BoolVar(&opts.mockSink, "mock-sink", false, "Indicates whether the Serverless is a mock sink")

	return cmd
}

type quicServerHandler struct {
	serverlessHandle plugin.Symbol
	mockSink         bool
}

func (s quicServerHandler) Listen() error {
	return nil
}

func (s quicServerHandler) Read(st quic.Stream) error {
	stream := dispatcher.Dispatcher(s.serverlessHandle, rx.FromReaderWithY3(st))

	y3codec := y3.NewCodec(0x10)

	go func() {
		for customer := range stream.Observe() {
			if customer.Error() {
				fmt.Println(customer.E.Error())
			} else if customer.V != nil {
				// HACK
				if s.mockSink {
					st.Write([]byte("Finish sink!"))
				} else {
					// use Y3 codec to encode the data
					sendingBuf, _ := y3codec.Marshal(customer.V)
					st.Write(sendingBuf)
				}
			}
		}
	}()
	return nil
}
