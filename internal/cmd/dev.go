package cmd

import (
	"context"
	"fmt"
	"io"
	"log"
	"plugin"

	"github.com/reactivex/rxgo/v2"
	"github.com/spf13/cobra"
	"github.com/yomorun/yomo/internal/dispatcher"
	"github.com/yomorun/yomo/internal/mocker"
	"github.com/yomorun/yomo/internal/serverless"
	"github.com/yomorun/yomo/pkg/quic"
	"github.com/yomorun/yomo/pkg/rx"
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
		Short: "Dev a YoMo Serverless Function",
		Long:  "Dev a YoMo Serverless Function with mocking yomo-source data from YCloud.",
		Run: func(cmd *cobra.Command, args []string) {
			slHandler, err := buildAndLoadHandler(&opts.baseOptions, args)
			if err != nil {
				return
			}

			// serve the Serverless app
			endpoint := fmt.Sprintf("0.0.0.0:%d", opts.Port)
			quicHandler := &quicDevHandler{
				serverlessHandle: slHandler,
				serverAddr:       fmt.Sprintf("localhost:%d", opts.Port),
				readers:          make(chan io.Reader),
			}

			err = serverless.Run(endpoint, quicHandler)
			if err != nil {
				log.Print("Run the serverless failure with err: ", err)
			}
		},
	}

	cmd.Flags().StringVarP(&opts.Filename, "file-name", "f", "app.go", "Serverless function file (default is app.go)")
	cmd.Flags().IntVarP(&opts.Port, "port", "p", 4242, "Port is the port number of UDP host for Serverless function (default is 4242)")

	return cmd
}

type quicDevHandler struct {
	serverlessHandle plugin.Symbol
	serverAddr       string
	readers          chan io.Reader
}

func (s *quicDevHandler) Listen() error {
	err := mocker.EmitMockDataFromCloud(s.serverAddr)
	if err != nil {
		return err
	}
	rxstream := rx.FromReaderWithY3(s.readers)
	stream := dispatcher.Dispatcher(s.serverlessHandle, rxstream)
	go func() {
		for customer := range stream.Observe(rxgo.WithErrorStrategy(rxgo.ContinueOnError)) {
			if customer.Error() {
				fmt.Println(customer.E.Error())
			}
		}
	}()

	rxstream.Connect(context.Background())

	return nil
}

func (s *quicDevHandler) Read(st quic.Stream) error {
	s.readers <- st
	return nil
}
