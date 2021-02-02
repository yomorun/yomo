package cmd

import (
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"plugin"

	"github.com/spf13/cobra"
	"github.com/yomorun/y3-codec-golang"
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
				readers:          make(chan io.Reader),
				writers:          make([]io.Writer, 0),
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
	readers          chan io.Reader
	writers          []io.Writer
}

func (s *quicServerHandler) Listen() error {
	rxstream := rx.FromReaderWithY3(s.readers)

	stream := dispatcher.Dispatcher(s.serverlessHandle, rxstream)
	rxstream.Connect(context.Background())

	y3codec := y3.NewCodec(0x10)

	go func() {
		for customer := range stream.Observe() {
			if customer.Error() {
				fmt.Println(customer.E.Error())
			} else if customer.V != nil {
				index := rand.Intn(len(s.writers))

				if s.mockSink {
				loopmock:
					for i, w := range s.writers {
						if index == i {
							_, err := w.Write([]byte("Finish sink!"))
							if err != nil {
								index = rand.Intn(len(s.writers))
								break loopmock
							}
						} else {
							w.Write([]byte{0})
						}
					}

				} else {
					// use Y3 codec to encode the data
					sendingBuf, _ := y3codec.Marshal(customer.V)

				loop:
					for i, w := range s.writers {
						if index == i {
							_, err := w.Write(sendingBuf)
							if err != nil {
								index = rand.Intn(len(s.writers))
								break loop
							}
						} else {
							w.Write([]byte{0})
						}
					}
				}
			}
		}
	}()

	return nil
}

func (s *quicServerHandler) Read(st quic.Stream) error {
	s.readers <- st
	s.writers = append(s.writers, st)
	return nil
}
