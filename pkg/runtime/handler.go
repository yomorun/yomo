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
	"github.com/yomorun/yomo/pkg/decoder"
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
		zipperMap:        sync.Map{},
		zipperSenders:    make([]serverless.GetSinkFunc, 0),
		zipperReceiver:   make(chan io.Reader),
	}
	return handler
}

type quicHandler struct {
	serverlessConfig *WorkflowConfig
	meshConfigURL    string
	connMap          sync.Map
	source           chan io.Reader
	zipperMap        sync.Map // the stream map for downstream zippers.
	zipperSenders    []serverless.GetSinkFunc
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
			s.buildZipperSenders()
		}()
	}

	return nil
}

func (s *quicHandler) Read(id int64, sess quic.Session, st quic.Stream) error {
	s.mutex.Lock()

	if c, ok := s.connMap.Load(id); ok {
		// the conn exists, reads new stream.
		c := c.(*ServerConn)
		if c.conn.Type == quic.ConnTypeSource {
			s.source <- st
		} else if c.conn.Type == quic.ConnTypeZipperSender {
			s.zipperReceiver <- st
		} else {
			c.conn.Stream = st
		}
	} else {
		// init
		conn := NewServerConn(sess, st, s.serverlessConfig)
		conn.onClosed = func() {
			s.connMap.Delete(id)
		}
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
					for _, sender := range s.zipperSenders {
						if sender == nil {
							continue
						}

						go func(f serverless.GetSinkFunc, buf []byte) {
							writer, cancel := f()
							if writer == nil {
								return
							}
							// send data to donwstream zippers
							_, err := writer.Write(value)
							if err != nil {
								log.Printf("❌ [Zipper Sender] sent data to downstream zipper failed: %s", err.Error())
								cancel()
							}
						}(sender, value)
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
				fd := decoder.NewFrameDecoder(receiver)
				for {
					buf, err := fd.Read(false)
					if err != nil {
						break
					} else {
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
							}(sink, buf)
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

// buildZipperSenders builds Zipper-Senders from edge-mesh config center.
func (s *quicHandler) buildZipperSenders() error {
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
			s.mutex.Lock()
			sender := s.createZipperSender(conf)
			s.zipperSenders = append(s.zipperSenders, sender)
			s.mutex.Unlock()
		}(conf)
	}

	return nil
}

// createZipperSender creates a zipper sender.
func (s *quicHandler) createZipperSender(conf zipperServerConf) serverless.GetSinkFunc {
	f := func() (io.Writer, serverless.CancelFunc) {
		if writer, ok := s.zipperMap.Load(conf.Name); ok {
			cli, ok := writer.(client.ZipperSenderClient)
			if ok {
				return cli, s.cancelZipperSender(conf)
			}
			return nil, s.cancelZipperSender(conf)
		}

		// Reset zipper in map
		s.zipperMap.Store(conf.Name, nil)

		// connect to downstream zipper
		cli, err := client.NewZipperSender(s.serverlessConfig.Name).
			Connect(conf.Host, conf.Port)
		if err != nil {
			cli.Retry()
		}

		s.zipperMap.Store(conf.Name, cli)
		return cli, s.cancelZipperSender(conf)
	}

	return f
}

// cancelZipperSender removes the zipper sender from `zipperMap`.
func (s *quicHandler) cancelZipperSender(conf zipperServerConf) func() {
	f := func() {
		s.zipperMap.Delete(conf.Name)
	}
	return f
}
