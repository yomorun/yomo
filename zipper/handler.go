package zipper

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/yomorun/yomo/core/quic"
	"github.com/yomorun/yomo/internal/core"
	"github.com/yomorun/yomo/internal/frame"
	"github.com/yomorun/yomo/logger"
)

type streamFuncWithCancel struct {
	addr    string
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
	source           chan quic.Stream
	zipperMap        sync.Map // the stream map for downstream YoMo-Zippers.
	zipperSenders    []GetSenderFunc
	zipperReceiver   chan quic.Stream
	mutex            sync.RWMutex
	onReceivedData   func(buf []byte) // the callback function when the data is received.
}

func newQuicHandler(conf *WorkflowConfig, meshConfURL string) *quicHandler {
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

func (s *quicHandler) Read(addr string, sess quic.Session, st quic.Stream) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// the connection exists
	if c, ok := s.connMap.Load(addr); ok {
		c := c.(*Conn)
		if c.Conn.Type == core.ConnTypeSource {
			s.source <- st
		} else if c.Conn.Type == core.ConnTypeUpstreamZipper {
			s.zipperReceiver <- st
		}

		return nil
	}

	// init a new connection.
	svrConn := NewConn(addr, sess, st, s.serverlessConfig)
	svrConn.onClosed = func() {
		s.connMap.Delete(addr)
	}
	s.connMap.Store(addr, svrConn)
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
			dataCh := DispatcherWithFunc(ctx, sfns, item)

			go func() {
				defer cancel()

				for data := range dataCh {
					logger.Debug("[zipper] receive data after running all Stream Functions, will drop it.", "data", logger.BytesString(data))
					// call the `onReceivedData` callback function.
					if s.onReceivedData != nil {
						s.onReceivedData(data)
					}

					// Upstream YoMo-Zippers
					for _, sender := range s.zipperSenders {
						if sender == nil {
							continue
						}

						txid := strconv.FormatInt(time.Now().UnixNano(), 10)
						frame := frame.NewDataFrame(txid)
						// playload frame
						// TODO: tag id
						frame.SetCarriage(0x12, data)
						go sendDataToDownstream(sender, frame, "[Upstream YoMo-Zipper] sent frame to downstream YoMo-Zipper Receiver.", "❌ [Upstream YoMo-Zipper] sent frame to downstream YoMo-Zipper Receiver failed.")
					}
				}
			}()
		}
	}
}

// receiveDataFromZipperSenders receives data from `Upstream YoMo-Zippers`.
func (s *quicHandler) receiveDataFromZipperSenders() {
	for {
		select {
		case receiver, ok := <-s.zipperReceiver:
			if !ok {
				return
			}

			ctx, cancel := context.WithCancel(context.Background())
			sfns := getStreamFuncs(s.serverlessConfig, &s.connMap)
			dataCh := DispatcherWithFunc(ctx, sfns, receiver)

			go func() {
				defer cancel()

				for data := range dataCh {
					logger.Debug("[YoMo-Zipper Receiver] receive data after running all Stream Functions, will drop it.", "data", logger.BytesString(data))
				}
			}()
		}
	}
}

// sendDataToDownstream sends data to `downstream`.
func sendDataToDownstream(sf GetSenderFunc, frame frame.Frame, succssMsg string, errMsg string) {
	for {
		name, writer, cancel := sf()
		if writer == nil {
			logger.Debug("[zipper] the downstream writer is nil", "name", name)
			break
		} else {
			data := frame.Encode()
			_, err := writer.Write(data)
			if err != nil {
				logger.Error(errMsg, "name", name, "frame", logger.BytesString(data), "err", err)
				cancel()
			} else {
				logger.Debug(succssMsg, "name", name, "frame", logger.BytesString(data))
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
		funcs = append(funcs, createStreamFunc(app, connMap, core.ConnTypeStreamFunction))
	}

	return funcs
}

var streamFuncCache = sync.Map{}           // the cache for all connections by name.
var newStreamFuncSessionCache = sync.Map{} // the cache for new connection channel by name.

// createStreamFunc creates a `GetStreamFunc` for `Stream Function`.
func createStreamFunc(app App, connMap *sync.Map, connType core.ConnectionType) GetStreamFunc {
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
				addr:    conn.Addr,
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
func cancelStreamFunc(name string, conn *Conn, connMap *sync.Map, addr string) func() {
	f := func() {
		clearStreamFuncCache(name)
		conn.Close()
		connMap.Delete(addr)
	}
	return f
}

// clearStreamFuncCache clear the local stream-function cache.
func clearStreamFuncCache(name string) {
	streamFuncCache.Delete(name)
}

// IsMatched indicates if the connection is matched.
func findConn(app App, connMap *sync.Map, connType core.ConnectionType) map[string]*Conn {
	results := make(map[string]*Conn)
	connMap.Range(func(key, value interface{}) bool {
		c := value.(*Conn)
		if c.Conn.Name == app.Name && c.Conn.Type == connType {
			results[key.(string)] = c
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

// buildZipperSenders builds Upstream YoMo-Zippers from edge-mesh config center.
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

// createZipperSender creates a Upstream YoMo-Zipper.
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
			logger.Error("[Upstream YoMo-Zipper] connect to downstream YoMo-Zipper failed, will retry...", "conf", conf, "err", err)
			cli.Retry()
		}

		s.zipperMap.Store(conf.Name, cli)
		logger.Printf("[Upstream YoMo-Zipper] Connected to downstream YoMo-Zipper %s, addr %s:%d", conf.Name, conf.Host, conf.Port)
		return conf.Name, cli, s.cancelZipperSender(conf)
	}

	return f
}

// cancelZipperSender removes the Upstream YoMo-Zipper from `zipperMap`.
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
		if c.Conn.Name == name {
			conn = c.Conn
			return false
		}
		return true
	})
	return conn
}
