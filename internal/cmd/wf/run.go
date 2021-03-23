package wf

// import (
// 	"fmt"
// 	"io"
// 	"log"
// 	"time"

// 	"github.com/spf13/cobra"
// 	"github.com/yomorun/yomo/internal/conf"
// 	"github.com/yomorun/yomo/internal/dispatcher"
// 	"github.com/yomorun/yomo/internal/workflow"
// 	"github.com/yomorun/yomo/pkg/quic"
// )

// // RunOptions are the options for run command.
// type RunOptions struct {
// 	baseOptions
// }

// // NewCmdRun creates a new command run.
// func NewCmdRun() *cobra.Command {
// 	var opts = &RunOptions{}

// 	var cmd = &cobra.Command{
// 		Use:   "run",
// 		Short: "Run a YoMo Serverless Function",
// 		Long:  "Run a YoMo Serverless Function",
// 		Run: func(cmd *cobra.Command, args []string) {
// 			conf, err := parseConfig(&opts.baseOptions, args)
// 			if err != nil {
// 				log.Print("❌ ", err)
// 				return
// 			}
// 			printZipperConf(conf)

// 			quicHandler := &quicHandler{
// 				serverlessConfig: conf,
// 				mergeChan:        make(chan []byte, 20),
// 			}

// 			endpoint := fmt.Sprintf("0.0.0.0:%d", conf.Port)

// 			log.Print("Running YoMo workflow...")
// 			err = workflow.Run(endpoint, quicHandler)
// 			if err != nil {
// 				log.Print("❌ ", err)
// 				return
// 			}
// 		},
// 	}

// 	cmd.Flags().StringVarP(&opts.Config, "config", "c", "workflow.yaml", "Workflow config file (default is workflow.yaml)")

// 	return cmd
// }

// type quicHandler struct {
// 	serverlessConfig *conf.WorkflowConfig
// 	mergeChan        chan []byte
// }

// func (s *quicHandler) Listen() error {

// 	return nil
// }

// func (s *quicHandler) Read(st quic.Stream) error {
// 	id := time.Now().UnixNano()
// 	flows, sinks := workflow.Build(s.serverlessConfig, id)

// 	stream := dispatcher.DispatcherWithFunc(flows, st)

// 	go func() {
// 		for customer := range stream.Observe() {
// 			if customer.Error() {
// 				fmt.Println(customer.E.Error())
// 				continue
// 			}
// 			value := customer.V.([]byte)
// 			if len(value) == 1 && value[0] == byte(0) {
// 				continue
// 			}

// 			for _, sink := range sinks {
// 				go func(_sink func() (io.Writer, func()), buf []byte) {
// 					writer, cancel := _sink()
// 					if writer != nil {
// 						_, err := writer.Write(buf)
// 						if err != nil {
// 							cancel()
// 						}

// 					}
// 				}(sink, value)
// 			}
// 		}
// 	}()

// 	return nil
// }
