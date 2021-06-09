package runtime

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"

	"github.com/reactivex/rxgo/v2"
	"github.com/yomorun/yomo/pkg/client"
	"github.com/yomorun/yomo/pkg/quic"
	"github.com/yomorun/yomo/pkg/serverless"
)

// NewServerHandler inits a new ServerHandler
func NewServerHandler(conf *WorkflowConfig, meshConfURL string) quic.ServerHandler {
	handler := &quicHandler{
		serverlessConfig: conf,
		meshConfigURL:    meshConfURL,
		connMap:          sync.Map{},
		source:           make(chan io.Reader),
		zipperSenders:    make([]io.Writer, 0),
		zipperReceiver:   make(chan io.Reader),
	}
	return handler
}

type quicHandler struct {
	serverlessConfig *WorkflowConfig
	meshConfigURL    string
	connMap          sync.Map
	source           chan io.Reader
	zipperSenders    []io.Writer
	zipperReceiver   chan io.Reader
	mutex            sync.RWMutex
}

func (s *quicHandler) Listen() error {
	go func() {
		s.receiveDataFromSources()
	}()

	go func() {
		s.receiveDataFromZipperSenders()
	}()

	if s.meshConfigURL != "" {
		go func() {
			s.getZipperSenders()
		}()
	}

	return nil
}

func (s *quicHandler) Read(id int64, sess quic.Session, st quic.Stream) error {
	s.mutex.Lock()

	if c, ok := s.connMap.Load(id); ok {
		// the conn exists, reads new stream.
		conn := c.(Conn)
		conn.OnRead(st, func() {
			streamType := conn.GetStreamType()
			if streamType == StreamTypeSource {
				s.source <- st
			} else if streamType == StreamTypeZipperSender {
				s.zipperReceiver <- st
			}
		})
	} else {
		// init
		conn := NewConn(sess, st, s.serverlessConfig)
		conn.OnClosed(func() {
			s.connMap.Delete(id)
		})
		s.connMap.Store(id, conn)
	}
	s.mutex.Unlock()
	return nil
}

// receiveDataFromSources receives the data from `YoMo-Sources`.
func (s *quicHandler) receiveDataFromSources() {
	for {
		select {
		case item, ok := <-s.source:
			if !ok {
				return
			}

			// one stream for each flows/sinks.
			flows, sinks := Build(s.serverlessConfig, &s.connMap)
			stream := DispatcherWithFunc(flows, item)

			go func() {
				for customer := range stream.Observe(rxgo.WithErrorStrategy(rxgo.ContinueOnError)) {
					if customer.Error() {
						fmt.Println(customer.E.Error())
						continue
					}

					value := customer.V.([]byte)

					// sinks
					for _, sink := range sinks {
						go func(sf serverless.GetSinkFunc, buf []byte) {
							writer, cancel := sf()

							if writer != nil {
								_, err := writer.Write(buf)
								if err != nil {
									cancel()
								}
							}
						}(sink, value)
					}

					// Zipper-Senders
					for i, sender := range s.zipperSenders {
						if sender == nil {
							continue
						}

						go func(w io.Writer, buf []byte, index int) {
							// send data to donwstream zippers
							_, err := w.Write(value)
							if err != nil {
								log.Printf("❌ [Zipper Sender] sent data to downstream zipper failed: %s", err.Error())
							}
						}(sender, value, i)
					}
				}
			}()
		}
	}
}

// receiveDataFromZipperSenders receives data from `YoMo-Zipper Senders`.
func (s *quicHandler) receiveDataFromZipperSenders() {
	for {
		select {
		case receiver, ok := <-s.zipperReceiver:
			if !ok {
				return
			}

			sinks := GetSinks(s.serverlessConfig, &s.connMap)
			if len(sinks) == 0 {
				continue
			}

			go func() {
				for {
					buf := make([]byte, 3*1024)
					n, err := receiver.Read(buf)
					if err != nil {
						break
					} else {
						value := buf[:n]
						// send data to sinks
						for _, sink := range sinks {
							go func(sf serverless.GetSinkFunc, buf []byte) {
								writer, cancel := sf()

								if writer != nil {
									_, err := writer.Write(buf)
									if err != nil {
										cancel()
									}
								}
							}(sink, value)
						}
					}
				}
			}()
		}
	}
}

// zipperServerConf represents the config of zipper servers
type zipperServerConf struct {
	Name string `json:"name"`
	Host string `json:"host"`
	Port int    `json:"port"`
}

// getZipperSenders connects to downstream zippers and get Zipper-Senders.
func (s *quicHandler) getZipperSenders() error {
	log.Print("Connecting to downstream zippers...")

	// download mesh conf
	res, err := http.Get(s.meshConfigURL)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	decoder := json.NewDecoder(res.Body)
	var configs []zipperServerConf
	err = decoder.Decode(&configs)
	if err != nil {
		return err
	}

	if len(configs) == 0 {
		return nil
	}

	for _, conf := range configs {
		if conf.Name == s.serverlessConfig.Name {
			// skip current zipper, only need to connect other zippers in edge-mesh.
			continue
		}

		go func(conf zipperServerConf) {
			cli, err := client.NewZipperSender(s.serverlessConfig.Name).
				Connect(conf.Host, conf.Port)
			if err != nil {
				cli.Retry()
			}

			s.mutex.Lock()
			s.zipperSenders = append(s.zipperSenders, cli)
			s.mutex.Unlock()
		}(conf)
	}

	return nil
}
