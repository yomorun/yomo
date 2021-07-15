package runtime

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/reactivex/rxgo/v2"
	"github.com/yomorun/yomo/pkg/client"
	"github.com/yomorun/yomo/pkg/decoder"
	"github.com/yomorun/yomo/pkg/logger"
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
			err := s.buildZipperSenders()
			if err != nil {
				logger.Error("❌ Downloaded the Mesh config failed.", "err", err)
			}
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
		svrConn := NewServerConn(sess, st, s.serverlessConfig)
		svrConn.onClosed = func() {
			s.connMap.Delete(id)
		}
		svrConn.isNewAppAvailable = func() bool {
			isNewAvailable := false
			// check if any new app (same name and type) is in connMap.
			s.connMap.Range(func(key, value interface{}) bool {
				c := value.(*ServerConn)
				if c.conn.Name == svrConn.conn.Name && c.conn.Type == svrConn.conn.Type && key.(int64) > id {
					isNewAvailable = true
					return false
				}
				return true
			})

			return isNewAvailable
		}
		s.connMap.Store(id, svrConn)
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
						go sendDataToSink(sink, value, "Zipper sent frame to sink", "❌ Zipper sent frame to sink failed.")
					}

					// Zipper-Senders
					for _, sender := range s.zipperSenders {
						if sender == nil {
							continue
						}

						go sendDataToSink(sender, value, "[Zipper Sender] sent frame to downstream zipper.", "❌ [Zipper Sender] sent frame to downstream zipper failed.")
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

			go func() {
				fd := decoder.NewFrameDecoder(receiver)
				for {
					buf, err := fd.Read(true)
					if err != nil {
						logger.Error("❌ [Zipper Receiver] received data from upstream zipper failed.", "err", err)
						break
					} else {
						logger.Debug("[Zipper Receiver] received frame from upstream zipper.", "frame", logger.BytesString(buf))

						if len(sinks) == 0 {
							logger.Warn("[Zipper Receiver] no sinks are available to receive the data.")
						} else {
							// send data to sinks
							for _, sink := range sinks {
								go sendDataToSink(sink, buf, "[Zipper Receiver] sent frame to sink.", "❌ [Zipper Receiver] sent frame to sink failed.")
							}
						}
					}
				}
			}()
		}
	}
}

func sendDataToSink(sf serverless.GetSinkFunc, buf []byte, succssMsg string, errMsg string) {
	for {
		writer, cancel := sf()
		if writer == nil {
			time.Sleep(200 * time.Millisecond)
		} else {
			_, err := writer.Write(buf)
			if err != nil {
				logger.Error(errMsg, "frame", logger.BytesString(buf), "err", err)
				cancel()
			} else {
				logger.Debug(succssMsg, "frame", logger.BytesString(buf))
				break
			}
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
	logger.Print("Downloading Mesh config...")

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

	logger.Print("✅ Successfully downloaded the Mesh config. ", configs)

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
			logger.Error("[Zipper Sender] connect to Zipper-Receiver failed, will retry...", "conf", conf, "err", err)
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
