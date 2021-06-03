package wf

import (
	"fmt"
	"log"
	"sync"

	"github.com/reactivex/rxgo/v2"
	"github.com/spf13/cobra"
	"github.com/yomorun/yomo/internal/conf"
	"github.com/yomorun/yomo/internal/dispatcher"
	"github.com/yomorun/yomo/internal/mocker"
	"github.com/yomorun/yomo/internal/workflow"
	"github.com/yomorun/yomo/pkg/quic"
	"github.com/yomorun/yomo/pkg/yomo"
)

// DevOptions are the options for dev command.
type DevOptions struct {
	baseOptions
}

// NewCmdDev creates a new command dev.
func NewCmdDev() *cobra.Command {
	var opts = &DevOptions{}

	var cmd = &cobra.Command{
		Use:   "dev",
		Short: "Dev a YoMo Serverless Function",
		Long:  "Dev a YoMo Serverless Function with mocking yomo-source data from YCloud.",
		Run: func(cmd *cobra.Command, args []string) {
			conf, err := parseConfig(&opts.baseOptions, args)
			if err != nil {
				log.Print("❌ ", err)
				return
			}
			printZipperConf(conf)

			log.Print("Running YoMo workflow...")
			endpoint := fmt.Sprintf("0.0.0.0:%d", conf.Port)

			quicHandler := &quicDevHandler{
				serverlessConfig: conf,
				serverAddr:       fmt.Sprintf("localhost:%d", conf.Port),
				connMap:          map[int64]*workflow.QuicConn{},
				source:           make(chan quic.Stream),
			}

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

type quicDevHandler struct {
	serverlessConfig *conf.WorkflowConfig
	serverAddr       string
	connMap          map[int64]*workflow.QuicConn
	source           chan quic.Stream
	mutex            sync.RWMutex
}

func (s *quicDevHandler) Listen() error {
	go func() {
		for {
			select {
			case item, ok := <-s.source:
				if !ok {
					return
				}

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
							go func(_sink yomo.SinkFunc, buf []byte) {
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

	go mocker.EmitMockDataFromCloud(s.serverAddr)

	return nil
}

func (s *quicDevHandler) Read(id int64, sess quic.Session, st quic.Stream) error {
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
