package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"sync"

	"github.com/reactivex/rxgo/v2"
	"github.com/yomorun/yomo/core/quic"
	"github.com/yomorun/yomo/internal/frame"
	"github.com/yomorun/yomo/logger"
)

type (
	// CancelFunc represents the function for cancellation.
	CancelFunc func()

	// GetStreamFunc represents the function to get stream function (former flow/sink).
	GetStreamFunc func() (string, quic.Session, CancelFunc)

	// GetSenderFunc represents the function to get YoMo-Sender.
	GetSenderFunc func() (string, io.Writer, CancelFunc)
)

// NewServerHandler inits a new ServerHandler
func NewServerHandler(conf *Config, meshConfURL string) quic.ServerHandler {
	return newQuicHandler(conf, meshConfURL)
}

type quicHandler struct {
	serverlessConfig *Config
	meshConfigURL    string
	connMap          sync.Map
	source           chan quic.Stream
	zipperMap        sync.Map // the stream map for downstream YoMo-Zippers.
	zipperSenders    []GetSenderFunc
	zipperReceiver   chan quic.Stream
	mutex            sync.RWMutex
	onReceivedData   func(buf []byte) // the callback function when the data is received.
}

func newQuicHandler(conf *Config, meshConfURL string) *quicHandler {
	return &quicHandler{
		serverlessConfig: conf,
		meshConfigURL:    meshConfURL,
		connMap:          sync.Map{},
		source:           make(chan quic.Stream),
		zipperMap:        sync.Map{},
		zipperSenders:    make([]GetSenderFunc, 0),
		zipperReceiver:   make(chan quic.Stream),
	}
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
				logger.Debug("❌ Download the mesh config failed.", "err", err)
			}
		}()
	}

	return nil
}

func (s *quicHandler) Read(id int64, sess quic.Session, st quic.Stream) error {
	// the connection exists
	if c, ok := s.connMap.Load(id); ok {
		s.mutex.Lock()
		c := c.(*Conn)
		if c.conn.Type == quic.ConnTypeSource {
			s.source <- st
		} else if c.conn.Type == quic.ConnTypeZipperSender {
			s.zipperReceiver <- st
		}

		s.mutex.Unlock()
		return nil
	}

	// init a new connection.
	s.mutex.Lock()
	svrConn := NewConn(sess, st, s.serverlessConfig)
	svrConn.onClosed = func() {
		s.connMap.Delete(id)
	}
	svrConn.isNewAppAvailable = func() bool {
		isNewAvailable := false
		// check if any new app (same name and type) is available in connMap.
		s.connMap.Range(func(key, value interface{}) bool {
			c := value.(*Conn)
			if c.conn.Name == svrConn.conn.Name && c.conn.Type == svrConn.conn.Type && key.(int64) > id {
				isNewAvailable = true
				return false
			}
			return true
		})

		return isNewAvailable
	}
	s.connMap.Store(id, svrConn)
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

			ctx, cancel := context.WithCancel(context.Background())
			sfns := getStreamFuncs(s.serverlessConfig, &s.connMap)
			stream := DispatcherWithFunc(ctx, sfns, item)

			go func() {
				defer cancel()

				for item := range stream.Observe(rxgo.WithErrorStrategy(rxgo.ContinueOnError)) {
					if item.Error() {
						logger.Error("[zipper] receive an error when running Stream Function.", "err", item.E.Error())
						continue
					}

					f, ok := item.V.(frame.DataFrame)
					if !ok {
						logger.Debug("[zipper] the type of item.V is not a Frame", "type", reflect.TypeOf(item.V))
						continue
					}
					logger.Debug("[zipper] receive data after running all Stream Functions, will drop it.", "data", logger.BytesString(f.Encode()))
					// call the `onReceivedData` callback function.
					if s.onReceivedData != nil {
						s.onReceivedData(f.GetCarriage())
					}

					// YoMo-Zipper-Senders
					for _, sender := range s.zipperSenders {
						if sender == nil {
							continue
						}

						go sendDataToDownstream(sender, f, "[YoMo-Zipper Sender] sent frame to downstream YoMo-Zipper Receiver.", "❌ [YoMo-Zipper Sender] sent frame to downstream YoMo-Zipper Receiver failed.")
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

			ctx, cancel := context.WithCancel(context.Background())
			sfns := getStreamFuncs(s.serverlessConfig, &s.connMap)
			stream := DispatcherWithFunc(ctx, sfns, receiver)

			go func() {
				defer cancel()

				for customer := range stream.Observe(rxgo.WithErrorStrategy(rxgo.ContinueOnError)) {
					if customer.Error() {
						fmt.Println(customer.E.Error())
						continue
					}
				}
			}()
		}
	}
}

// sendDataToDownstream sends data to `downstream`.
func sendDataToDownstream(sf GetSenderFunc, f frame.DataFrame, succssMsg string, errMsg string) {
	for {
		name, writer, cancel := sf()
		if writer == nil {
			logger.Debug("[zipper] the downstream writer is nil", "name", name)
			break
		} else {
			_, err := writer.Write(f.GetCarriage())
			if err != nil {
				logger.Error(errMsg, "name", name, "frame", logger.BytesString(f.Encode()), "err", err)
				cancel()
			} else {
				logger.Debug(succssMsg, "name", name, "frame", logger.BytesString(f.Encode()))
				break
			}
		}
	}
}

// getStreamFuncs gets stream functions by config (.yaml).
// It will create one stream for each function.
func getStreamFuncs(wfConf *Config, connMap *sync.Map) []GetStreamFunc {
	//init workflow
	funcs := make([]GetStreamFunc, 0)

	for _, app := range wfConf.Functions {
		funcs = append(funcs, createStreamFunc(app, connMap, quic.ConnTypeStreamFunction))
	}

	return funcs
}

// createStreamFunc creates a `GetStreamFunc` for `Stream Function`.
func createStreamFunc(app Function, connMap *sync.Map, connType string) GetStreamFunc {
	f := func() (string, quic.Session, CancelFunc) {
		id, c := findConn(app, connMap, connType)

		if c == nil {
			return app.Name, nil, func() {}
		}

		if c.Session != nil {
			return app.Name, c.Session, cancelStreamFunc(c, connMap, id)
		}

		return app.Name, nil, func() {}
	}

	return f
}

// cancelStreamFunc close the connection of Stream Function.
func cancelStreamFunc(conn *Conn, connMap *sync.Map, id int64) func() {
	f := func() {
		conn.Close()
		connMap.Delete(id)
	}
	return f
}

// IsMatched indicates if the connection is matched.
func findConn(app Function, connMap *sync.Map, connType string) (int64, *Conn) {
	var conn *Conn
	var id int64
	connMap.Range(func(key, value interface{}) bool {
		c := value.(*Conn)
		if c.conn.Name == app.Name && c.conn.Type == connType {
			conn = c
			id = key.(int64)

			return false
		}
		return true
	})

	return id, conn
}

// zipperConf represents the config of yomo-zipper
type zipperConf struct {
	Name string `json:"name"`
	Host string `json:"host"`
	Port int    `json:"port"`
}

// buildZipperSenders builds YoMo-Zipper-Senders from edge-mesh config center.
func (s *quicHandler) buildZipperSenders() error {
	logger.Print("Downloading mesh config...")

	// download mesh conf
	res, err := http.Get(s.meshConfigURL)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	decoder := json.NewDecoder(res.Body)
	var configs []zipperConf
	err = decoder.Decode(&configs)
	if err != nil {
		return err
	}

	logger.Print("✅ Successfully downloaded the Mesh config. ", configs)

	if len(configs) == 0 {
		return nil
	}

	for _, conf := range configs {
		go func(conf zipperConf) {
			s.mutex.Lock()
			sender := s.createZipperSender(conf)
			s.zipperSenders = append(s.zipperSenders, sender)
			s.mutex.Unlock()
		}(conf)
	}

	return nil
}

// createZipperSender creates a YoMo-Zipper sender.
func (s *quicHandler) createZipperSender(conf zipperConf) GetSenderFunc {
	f := func() (string, io.Writer, CancelFunc) {
		if writer, ok := s.zipperMap.Load(conf.Name); ok {
			cli, ok := writer.(SenderClient)
			if ok {
				return conf.Name, cli, s.cancelZipperSender(conf)
			}
			return conf.Name, nil, s.cancelZipperSender(conf)
		}

		// Reset YoMo-Zipper in map
		s.zipperMap.Store(conf.Name, nil)

		// connect to downstream YoMo-Zipper
		cli, err := NewSender(s.serverlessConfig.Name).
			Connect(conf.Host, conf.Port)
		if err != nil {
			logger.Error("[YoMo-Zipper Sender] connect to YoMo-Zipper Receiver failed, will retry...", "conf", conf, "err", err)
			cli.Retry()
		}

		s.zipperMap.Store(conf.Name, cli)
		return conf.Name, cli, s.cancelZipperSender(conf)
	}

	return f
}

// cancelZipperSender removes the YoMo-Zipper sender from `zipperMap`.
func (s *quicHandler) cancelZipperSender(conf zipperConf) func() {
	f := func() {
		s.zipperMap.Delete(conf.Name)
	}
	return f
}

func (s *quicHandler) getConn(name string) *quic.Conn {
	var conn *quic.Conn
	s.connMap.Range(func(key, value interface{}) bool {
		c := value.(*Conn)
		if c.conn.Name == name {
			conn = c.conn
			return false
		}
		return true
	})
	return conn
}
