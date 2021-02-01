package cmd

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"plugin"

	"github.com/spf13/cobra"
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
}

func (s *quicServerHandler) Listen() error {
	//var writers []io.Writer

	rxstream := rx.FromReaderWithY3(s.readers)
	//rxObserve := rxstream.Observe()

	// go func() {
	// 	for item := range rxObserve {
	// 		writers = append(writers, (item.V).(io.Writer))
	// 	}
	// }()

	stream := dispatcher.Dispatcher(s.serverlessHandle, rxstream)
	rxstream.Connect(context.Background())
	//	y3codec := y3.NewCodec(0x10)

	go func() {
		for customer := range stream.Observe() {
			if customer.Error() {
				fmt.Println(customer.E.Error())
			} else if customer.V != nil {
				//	index := rand.Intn(len(writers))
				// HACK
				// if s.mockSink {
				// 	_, err := writers[index].Write([]byte("Finish sink!"))
				// 	if err != nil {
				// 		for _, w := range writers {
				// 			_, err := w.Write([]byte("Finish sink!"))
				// 			if err == nil {
				// 				break
				// 			}
				// 		}
				// 	}

				// } else {
				// 	// use Y3 codec to encode the data
				// 	fmt.Println("==============1=============")
				// 	sendingBuf, _ := y3codec.Marshal(customer.V)
				// 	_, err := writers[index].Write(sendingBuf)
				// 	if err != nil {
				// 		for _, w := range writers {
				// 			_, err := w.Write(sendingBuf)
				// 			if err == nil {
				// 				break
				// 			}
				// 		}
				// 	}
				// }
			}
		}
	}()

	return nil
}

func (s *quicServerHandler) Read(st quic.Stream) error {
	s.readers <- st
	fmt.Println("===========================")
	return nil
}
