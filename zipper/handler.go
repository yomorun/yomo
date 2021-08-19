package zipper

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
	"github.com/yomorun/yomo/internal/decoder"
	"github.com/yomorun/yomo/internal/framing"
	"github.com/yomorun/yomo/logger"
)

type streamFuncWithCancel struct {
	session quic.Session
	cancel  CancelFunc
}

type (
	// CancelFunc represents the function for cancellation.
	CancelFunc func()

	// GetStreamFunc represents the function to get stream function (former flow/sink).
	GetStreamFunc func() (string, []streamFuncWithCancel)

	// GetSenderFunc represents the function to get YoMo-Sender.
	GetSenderFunc func() (string, io.Writer, CancelFunc)
)

// NewServerHandler inits a new ServerHandler
func NewServerHandler(conf *WorkflowConfig, meshConfURL string) quic.ServerHandler {
	return newQuicHandler(conf, meshConfURL)
}

type quicHandler struct {
	serverlessConfig *WorkflowConfig
	meshConfigURL    string
	connMap          sync.Map
	source           chan decoder.Reader
	zipperMap        sync.Map // the stream map for downstream YoMo-Zippers.
	zipperSenders    []GetSenderFunc
	zipperReceiver   chan decoder.Reader
	mutex            sync.RWMutex
	onReceivedData   func(buf []byte) // the callback function when the data is received.
}

func newQuicHandler(conf *WorkflowConfig, meshConfURL string) *quicHandler {
	return &quicHandler{
		serverlessConfig: conf,
		meshConfigURL:    meshConfURL,
		connMap:          sync.Map{},
		source:           make(chan decoder.Reader),
		zipperMap:        sync.Map{},
		zipperSenders:    make([]GetSenderFunc, 0),
		zipperReceiver:   make(chan decoder.Reader),
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
			s.source <- decoder.NewReader(st)
		} else if c.conn.Type == quic.ConnTypeZipperSender {
			s.zipperReceiver <- decoder.NewReader(st)
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

					data, ok := item.V.([]byte)
					if !ok {
						logger.Debug("[zipper] the type of item.V is not a []byte.", "type", reflect.TypeOf(item.V))
						continue
					}
					logger.Debug("[zipper] receive data after running all Stream Functions, will drop it.", "data", logger.BytesString(data))
					// call the `onReceivedData` callback function.
					if s.onReceivedData != nil {
						s.onReceivedData(data)
					}

					// YoMo-Zipper-Senders
					for _, sender := range s.zipperSenders {
						if sender == nil {
							continue
						}

						frame := framing.NewPayloadFrame(data)
						go sendDataToDownstream(sender, frame, "[YoMo-Zipper Sender] sent frame to downstream YoMo-Zipper Receiver.", "❌ [YoMo-Zipper Sender] sent frame to downstream YoMo-Zipper Receiver failed.")
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
func sendDataToDownstream(sf GetSenderFunc, frame framing.Frame, succssMsg string, errMsg string) {
	for {
		name, writer, cancel := sf()
		if writer == nil {
			logger.Debug("[zipper] the downstream writer is nil", "name", name)
			break
		} else {
			_, err := writer.Write(frame.Data())
			if err != nil {
				logger.Error(errMsg, "name", name, "frame", logger.BytesString(frame.Bytes()), "err", err)
				cancel()
			} else {
				logger.Debug(succssMsg, "name", name, "frame", logger.BytesString(frame.Bytes()))
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

var streamFuncCache = sync.Map{}           // the cache for all connections by name.
var newStreamFuncSessionCache = sync.Map{} // the cache for new connection channel by name.

// createStreamFunc creates a `GetStreamFunc` for `Stream Function`.
func createStreamFunc(app App, connMap *sync.Map, connType string) GetStreamFunc {
	f := func() (string, []streamFuncWithCancel) {
		// get from local cache.
		if funcs, ok := streamFuncCache.Load(app.Name); ok {
			return app.Name, funcs.([]streamFuncWithCancel)
		}

		// get from connMap.
		conns := findConn(app, connMap, connType)
		funcs := make([]streamFuncWithCancel, len(conns))

		if len(conns) == 0 {
			streamFuncCache.Store(app.Name, funcs)
			return app.Name, funcs
		}

		i := 0
		for id, conn := range conns {
			funcs[i] = streamFuncWithCancel{
				session: conn.Session,
				cancel:  cancelStreamFunc(app.Name, conn, connMap, id),
			}
			i++
		}

		streamFuncCache.Store(app.Name, funcs)
		return app.Name, funcs
	}

	return f
}

// cancelStreamFunc close the connection of Stream Function.
func cancelStreamFunc(name string, conn *Conn, connMap *sync.Map, id int64) func() {
	f := func() {
		clearStreamFuncCache(name)
		conn.Close()
		connMap.Delete(id)
	}
	return f
}

// clearStreamFuncCache clear the local stream-function cache.
func clearStreamFuncCache(name string) {
	streamFuncCache.Delete(name)
}

// IsMatched indicates if the connection is matched.
func findConn(app App, connMap *sync.Map, connType string) map[int64]*Conn {
	results := make(map[int64]*Conn)
	connMap.Range(func(key, value interface{}) bool {
		c := value.(*Conn)
		if c.conn.Name == app.Name && c.conn.Type == connType {
			results[key.(int64)] = c
		}
		return true
	})

	return results
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
