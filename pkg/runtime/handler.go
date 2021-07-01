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
		serverlessConfig:   conf,
		meshConfigURL:      meshConfURL,
		connMap:            sync.Map{},
		source:             make(chan io.Reader),
		outputConnectorMap: sync.Map{},
		zipperMap:          sync.Map{},
		zipperSenders:      make([]serverless.GetSenderFunc, 0),
		zipperReceiver:     make(chan io.Reader),
	}
	return handler
}

type quicHandler struct {
	serverlessConfig   *WorkflowConfig
	meshConfigURL      string
	connMap            sync.Map
	source             chan io.Reader
	outputConnectorMap sync.Map
	zipperMap          sync.Map // the stream map for downstream zippers.
	zipperSenders      []serverless.GetSenderFunc
	zipperReceiver     chan io.Reader
	mutex              sync.RWMutex
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

		svrConn.onGotAppType = func() {
			// output connector
			if svrConn.conn.Type == quic.ConnTypeOutputConnector {
				s.outputConnectorMap.Store(id, createOutputConnectorFunc(id, &s.connMap, &s.outputConnectorMap))
			}
		}
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

			// one stream for each stream functions.
			sfns := getStreamFuncs(s.serverlessConfig, &s.connMap)
			stream := DispatcherWithFunc(sfns, item)

			go func() {
				for customer := range stream.Observe(rxgo.WithErrorStrategy(rxgo.ContinueOnError)) {
					if customer.Error() {
						fmt.Println(customer.E.Error())
						continue
					}

					buf := customer.V.([]byte)

					// send data to `Output Connectors`
					s.outputConnectorMap.Range(func(key, value interface{}) bool {
						if value == nil {
							return true
						}

						sf, ok := value.(serverless.GetSenderFunc)
						if ok {
							go sendDataToConnector(sf, buf, "Zipper sent frame to sink", "❌ Zipper sent frame to sink failed.")
						}
						return true
					})

					// Zipper-Senders
					for _, sender := range s.zipperSenders {
						if sender == nil {
							continue
						}

						go sendDataToConnector(sender, buf, "[Zipper Sender] sent frame to downstream zipper.", "❌ [Zipper Sender] sent frame to downstream zipper failed.")
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

			go func() {
				fd := decoder.NewFrameDecoder(receiver)
				for {
					buf, err := fd.Read(false)
					if err != nil {
						break
					} else {
						// send data to `Output Connectors`
						s.outputConnectorMap.Range(func(key, value interface{}) bool {
							if value == nil {
								return true
							}

							sf, ok := value.(serverless.GetSenderFunc)
							if ok {
								go sendDataToConnector(sf, buf, "[Zipper Receiver] sent frame to sink.", "❌ [Zipper Receiver] sent frame to sink failed.")
							}
							return true
						})
					}
				}
			}()
		}
	}
}

func sendDataToConnector(sf serverless.GetSenderFunc, buf []byte, succssMsg string, errMsg string) {
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

// getStreamFuncs gets stream functions by config (.yaml).
// It will create one stream for each function.
func getStreamFuncs(wfConf *WorkflowConfig, connMap *sync.Map) []serverless.GetStreamFunc {
	//init workflow
	funcs := make([]serverless.GetStreamFunc, 0)

	for _, app := range wfConf.Functions {
		funcs = append(funcs, createStreamFunc(app, connMap, quic.ConnTypeStreamFunction))
	}

	return funcs
}

// createStreamFunc creates a `GetStreamFunc` for `Stream Function`.
func createStreamFunc(app App, connMap *sync.Map, connType string) serverless.GetStreamFunc {
	f := func() (io.ReadWriter, serverless.CancelFunc) {
		id, c := findConn(app, connMap, connType)

		if c == nil {
			return nil, func() {}
		} else if c.conn.Stream != nil {
			return c.conn.Stream, cancelStreamFunc(c, connMap, id)
		} else {
			c.SendSignalFunction()
			return nil, func() {}
		}
	}

	return f
}

func cancelStreamFunc(conn *ServerConn, connMap *sync.Map, id int64) func() {
	f := func() {
		conn.Close()
		connMap.Delete(id)
	}
	return f
}

// IsMatched indicates if the connection is matched.
func findConn(app App, connMap *sync.Map, connType string) (int64, *ServerConn) {
	var conn *ServerConn = nil
	var id int64 = 0
	connMap.Range(func(key, value interface{}) bool {
		c := value.(*ServerConn)
		if c.conn.Name == app.Name && c.conn.Type == connType {
			conn = c
			id = key.(int64)

			return false
		}
		return true
	})

	return id, conn
}

// createOutputConnectorFunc creates a `GetSenderFunc` for `Output Connector`.
func createOutputConnectorFunc(id int64, connMap *sync.Map, outputConnectorMap *sync.Map) serverless.GetSenderFunc {
	f := func() (io.Writer, serverless.CancelFunc) {
		value, ok := connMap.Load(id)

		if !ok {
			return nil, func() {}
		}

		c := value.(*ServerConn)

		if c.conn.Stream != nil {
			return c.conn.Stream, cancelOutputConnectorFunc(c, connMap, outputConnectorMap, id)
		} else {
			c.SendSignalFunction()
			return nil, func() {}
		}
	}

	return f
}

func cancelOutputConnectorFunc(conn *ServerConn, connMap *sync.Map, outputConnectorMap *sync.Map, id int64) func() {
	f := func() {
		conn.Close()
		connMap.Delete(id)
		outputConnectorMap.Delete(id)
	}
	return f
}

// zipperServerConf represents the config of zipper servers
type zipperServerConf struct {
	Name string `json:"name"`
	Host string `json:"host"`
	Port int    `json:"port"`
}

// buildZipperSenders builds Zipper-Senders from edge-mesh config center.
func (s *quicHandler) buildZipperSenders() error {
	logger.Print("Connecting to downstream zippers...")

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
func (s *quicHandler) createZipperSender(conf zipperServerConf) serverless.GetSenderFunc {
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
