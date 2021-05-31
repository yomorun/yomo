package wf

import (
	"fmt"
	"io"
	"log"
	"sync"

	"github.com/reactivex/rxgo/v2"
	"github.com/spf13/cobra"
	"github.com/yomorun/yomo/internal/conf"
	"github.com/yomorun/yomo/internal/dispatcher"
	"github.com/yomorun/yomo/internal/workflow"
	"github.com/yomorun/yomo/pkg/quic"
)

// RunOptions are the options for run command.
type RunOptions struct {
	baseOptions
}

// NewCmdRun creates a new command run.
func NewCmdRun() *cobra.Command {
	var opts = &RunOptions{}

	var cmd = &cobra.Command{
		Use:   "run",
		Short: "Run a YoMo Serverless Function",
		Long:  "Run a YoMo Serverless Function",
		Run: func(cmd *cobra.Command, args []string) {
			conf, err := parseConfig(&opts.baseOptions, args)
			if err != nil {
				log.Print("❌ ", err)
				return
			}
			printZipperConf(conf)

			quicHandler := &quicHandler{
				serverlessConfig: conf,
				connMap:          map[int64]*workflow.QuicConn{},
				source:           make(chan io.Reader),
			}

			endpoint := fmt.Sprintf("0.0.0.0:%d", conf.Port)

			log.Print("Running YoMo workflow...")
			err = workflow.Run(endpoint, quicHandler)
			if err != nil {
				log.Print("❌ ", err)
				return
			}
		},
	}

	cmd.Flags().StringVarP(&opts.Config, "config", "c", "workflow.yaml", "Workflow config file")

	return cmd
}

type quicHandler struct {
	serverlessConfig *conf.WorkflowConfig
	connMap          map[int64]*workflow.QuicConn
	source           chan io.Reader
	mutex            sync.RWMutex
}

func (s *quicHandler) Listen() error {
	go func() {
		for {
			select {
			case item, ok := <-s.source:
				if !ok {
					return
				}

				// one stream for each flows/sinks.
				flows, sinks := workflow.Build(s.serverlessConfig, &s.connMap)
				stream := dispatcher.DispatcherWithFunc(flows, item)

				go func() {
					for customer := range stream.Observe(rxgo.WithErrorStrategy(rxgo.ContinueOnError)) {
						if customer.Error() {
							fmt.Println(customer.E.Error())
							continue
						}

						value := customer.V.([]byte)

						for _, sink := range sinks {
							go func(_sink func() (io.Writer, func()), buf []byte) {
								writer, cancel := _sink()

								if writer != nil {
									_, err := writer.Write(buf)
									if err != nil {
										cancel()
									}
								}
							}(sink, value)
						}
					}
				}()
			}
		}
	}()
	return nil
}

func (s *quicHandler) Read(id int64, sess quic.Session, st quic.Stream) error {
	s.mutex.Lock()

	if conn, ok := s.connMap[id]; ok {
		if conn.StreamType == workflow.StreamTypeSource {
			s.source <- st
		} else {
			conn.Stream = st
		}
	} else {
		conn := &workflow.QuicConn{
			Session:    sess,
			Signal:     st,
			StreamType: "",
			Name:       "",
			Heartbeat:  make(chan byte),
			IsClosed:   false,
			Ready:      true,
		}
		conn.Init(s.serverlessConfig)
		s.connMap[id] = conn
	}
	s.mutex.Unlock()
	return nil
}
