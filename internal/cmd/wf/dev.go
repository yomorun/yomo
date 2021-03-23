package wf

import (
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/yomorun/yomo/internal/conf"
	"github.com/yomorun/yomo/internal/dispatcher"
	"github.com/yomorun/yomo/internal/mocker"
	"github.com/yomorun/yomo/internal/workflow"
	"github.com/yomorun/yomo/pkg/quic"
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
				build:            make(chan quic.Stream),
				index:            0,
				lastStream:       make(chan bool),
			}

			err = workflow.Run(endpoint, quicHandler)
			if err != nil {
				log.Print("❌ ", err)
				return
			}
		},
	}

	cmd.Flags().StringVarP(&opts.Config, "config", "c", "workflow.yaml", "Workflow config file (default is workflow.yaml)")

	return cmd
}

type quicDevHandler struct {
	serverlessConfig *conf.WorkflowConfig
	serverAddr       string
	connMap          map[int64]*workflow.QuicConn
	build            chan quic.Stream
	index            int
	mutex            sync.RWMutex
	lastStream       chan bool
}

func (s *quicDevHandler) Listen() error {
	go mocker.EmitMockDataFromCloud(s.serverAddr)

	go func() {
		for {
			select {
			case item, ok := <-s.build:
				if !ok {
					return
				}

				flows, sinks := workflow.Build(s.serverlessConfig, &s.connMap, &s.lastStream)
				stream := dispatcher.DispatcherWithFunc(flows, item)

				go func() {
					for customer := range stream.Observe() {
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

func (s *quicDevHandler) Read(id int64, sess quic.Session, st quic.Stream) error {
	s.mutex.Lock()
	if conn, ok := s.connMap[id]; ok {
		appName := ""
		appType := []byte{0}

		for i, v := range s.serverlessConfig.Sinks {
			if i == 0 {
				appName = v.Name
				appType = []byte{0, 1}
			}
		}

		for i, v := range s.serverlessConfig.Flows {
			if i == 0 {
				appName = v.Name
				appType = []byte{0, 0}
			}
		}
		// source : receivable is false
		if !conn.Receivable {
			conn.StreamType = "source"
			conn.Stream = append(conn.Stream, st)
			s.index++
			s.build <- st

			if s.index > 1 {
				go func() {
					var c *workflow.QuicConn = nil
				loop:
					for _, v := range s.connMap {
						if v.Name == appName {
							c = v
						}
					}
					if c == nil {
						time.Sleep(time.Second)
						goto loop
					} else {
						c.SendSignal(appType)
					}
				}()
			}

		} else if conn.StreamType == "flow" {
			conn.Stream = append(conn.Stream, st)
			if appName == conn.Name {
				s.index--
			}
		} else if conn.StreamType == "sink" {
			conn.Stream = append(conn.Stream, st)
			if appName == conn.Name {
				s.index--
			}
		}

		if s.index == 0 {
			s.lastStream <- true
		}
	} else {
		conn := &workflow.QuicConn{
			Session:    sess,
			Signal:     st,
			Receivable: false,
			Stream:     make([]quic.Stream, 0),
			StreamType: "",
			Name:       "",
			Heartbeat:  make(chan byte),
			IsClose:    false,
		}
		conn.Init()
		s.connMap[id] = conn
	}
	s.mutex.Unlock()
	return nil
}
