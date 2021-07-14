package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/reactivex/rxgo/v2"
	"github.com/yomorun/yomo/core/quic"
	"github.com/yomorun/yomo/internal/decoder"
	"github.com/yomorun/yomo/logger"
)

type (
	// CancelFunc represents the function for cancellation.
	CancelFunc func()

	// GetStreamFunc represents the function to get stream function (former flow/sink).
	GetStreamFunc func() (io.ReadWriter, CancelFunc)

	// GetSenderFunc represents the function to get YoMo-Sender.
	GetSenderFunc func() (io.Writer, CancelFunc)
)

// NewServerHandler inits a new ServerHandler
func NewServerHandler(conf *WorkflowConfig, meshConfURL string) quic.ServerHandler {
	handler := &quicHandler{
		serverlessConfig:   conf,
		meshConfigURL:      meshConfURL,
		connMap:            sync.Map{},
		source:             make(chan io.Reader),
		outputConnectorMap: sync.Map{},
		serverMap:          sync.Map{},
		serverSenders:      make([]GetSenderFunc, 0),
		serverReceiver:     make(chan io.Reader),
	}
	return handler
}

type quicHandler struct {
	serverlessConfig   *WorkflowConfig
	meshConfigURL      string
	connMap            sync.Map
	source             chan io.Reader
	outputConnectorMap sync.Map
	serverMap          sync.Map // the stream map for downstream yomo servers.
	serverSenders      []GetSenderFunc
	serverReceiver     chan io.Reader
	mutex              sync.RWMutex
}

func (s *quicHandler) Listen() error {
	go func() {
		s.receiveDataFromSources()
	}()

	go func() {
		s.receiveDataFromServerSenders()
	}()

	if s.meshConfigURL != "" {
		go func() {
			err := s.buildServerSenders()
			if err != nil {
				logger.Debug("❌ Download the mesh config failed.", "err", err)
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
		} else if c.conn.Type == quic.ConnTypeServerSender {
			s.serverReceiver <- st
		} else {
			c.conn.Stream = st
		}
	} else {
		// init conn.
		svrConn := NewServerConn(sess, st, s.serverlessConfig)
		svrConn.onClosed = func() {
			s.connMap.Delete(id)
		}
		svrConn.isNewAppAvailable = func() bool {
			isNewAvailable := false
			// check if any new app (same name and type) is available in connMap.
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

						sf, ok := value.(GetSenderFunc)
						if ok {
							go sendDataToConnector(sf, buf, "[YoMo-Server] sent frame to Output-Connector", "❌ [YoMo-Server] sent frame to Output-Connector failed.")
						}
						return true
					})

					// YoMo-Server-Senders
					for _, sender := range s.serverSenders {
						if sender == nil {
							continue
						}

						go sendDataToConnector(sender, buf, "[YoMo-Server Sender] sent frame to downstream YoMo-Server Receiver.", "❌ [YoMo-Server Sender] sent frame to downstream YoMo-Server Receiver failed.")
					}
				}
			}()
		}
	}
}

// receiveDataFromServerSenders receives data from `YoMo-Server Senders`.
func (s *quicHandler) receiveDataFromServerSenders() {
	for {
		select {
		case receiver, ok := <-s.serverReceiver:
			if !ok {
				return
			}

			go func() {
				fd := decoder.NewFrameDecoder(receiver)
				for {
					buf, err := fd.Read(true)
					if err != nil {
						logger.Error("❌ [YoMo-Server Receiver] received data from upstream YoMo-Server Sender failed.", "err", err)
						break
					} else {
						logger.Debug("[YoMo-Server Receiver] received frame from upstream YoMo-Server Sender.", "frame", logger.BytesString(buf))

						// send data to `Output Connectors`
						s.outputConnectorMap.Range(func(key, value interface{}) bool {
							if value == nil {
								return true
							}

							sf, ok := value.(GetSenderFunc)
							if ok {
								go sendDataToConnector(sf, buf, "[YoMo-Server Receiver] sent frame to Output-Connector.", "❌ [YoMo-Server Receiver] sent frame to Output-Connector failed.")
							}
							return true
						})
					}
				}
			}()
		}
	}
}

// sendDataToConnector sends data to `Output Connector`.
func sendDataToConnector(sf GetSenderFunc, buf []byte, succssMsg string, errMsg string) {
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
func getStreamFuncs(wfConf *WorkflowConfig, connMap *sync.Map) []GetStreamFunc {
	//init workflow
	funcs := make([]GetStreamFunc, 0)

	for _, app := range wfConf.Functions {
		funcs = append(funcs, createStreamFunc(app, connMap, quic.ConnTypeStreamFunction))
	}

	return funcs
}

// createStreamFunc creates a `GetStreamFunc` for `Stream Function`.
func createStreamFunc(app App, connMap *sync.Map, connType string) GetStreamFunc {
	f := func() (io.ReadWriter, CancelFunc) {
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

// cancelStreamFunc close the connection of Stream Function.
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
func createOutputConnectorFunc(id int64, connMap *sync.Map, outputConnectorMap *sync.Map) GetSenderFunc {
	f := func() (io.Writer, CancelFunc) {
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

// cancelOutputConnectorFunc close the connection of Output Connection.
func cancelOutputConnectorFunc(conn *ServerConn, connMap *sync.Map, outputConnectorMap *sync.Map, id int64) func() {
	f := func() {
		conn.Close()
		connMap.Delete(id)
		outputConnectorMap.Delete(id)
	}
	return f
}

// serverConf represents the config of yomo servers
type serverConf struct {
	Name string `json:"name"`
	Host string `json:"host"`
	Port int    `json:"port"`
}

// buildServerSenders builds YoMo-Server-Senders from edge-mesh config center.
func (s *quicHandler) buildServerSenders() error {
	logger.Print("Downloading mesh config...")

	// download mesh conf
	res, err := http.Get(s.meshConfigURL)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	decoder := json.NewDecoder(res.Body)
	var configs []serverConf
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
			// skip current yomo-server, only need to connect other yomo-servers in edge-mesh.
			continue
		}

		go func(conf serverConf) {
			s.mutex.Lock()
			sender := s.createServerSender(conf)
			s.serverSenders = append(s.serverSenders, sender)
			s.mutex.Unlock()
		}(conf)
	}

	return nil
}

// createServerSender creates a yomo-server sender.
func (s *quicHandler) createServerSender(conf serverConf) GetSenderFunc {
	f := func() (io.Writer, CancelFunc) {
		if writer, ok := s.serverMap.Load(conf.Name); ok {
			cli, ok := writer.(SenderClient)
			if ok {
				return cli, s.cancelServerSender(conf)
			}
			return nil, s.cancelServerSender(conf)
		}

		// Reset yomo-server in map
		s.serverMap.Store(conf.Name, nil)

		// connect to downstream yomo-server
		cli, err := NewSender(s.serverlessConfig.Name).
			Connect(conf.Host, conf.Port)
		if err != nil {
			logger.Error("[YoMo-Server Sender] connect to YoMo-Server Receiver failed, will retry...", "conf", conf, "err", err)
			cli.Retry()
		}

		s.serverMap.Store(conf.Name, cli)
		return cli, s.cancelServerSender(conf)
	}

	return f
}

// cancelServerSender removes the yomo-server sender from `serverMap`.
func (s *quicHandler) cancelServerSender(conf serverConf) func() {
	f := func() {
		s.serverMap.Delete(conf.Name)
	}
	return f
}
