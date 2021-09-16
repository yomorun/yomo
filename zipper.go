package yomo

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/yomorun/yomo/internal/core"
	"github.com/yomorun/yomo/internal/util"
	"github.com/yomorun/yomo/logger"
)

const (
	zipperLogPrefix = "\033[33m[yomo:zipper]\033[0m "
)

// Zipper is the orchestrator of yomo. There are two types of zipper:
// one is Upstream Zipper, which is used to connect to multiple downstream zippers,
// another one is Downstream Zipper (will call it as Zipper directly), which is used
// to connected by `Upstream Zipper`, `Source` and `Stream Function`
type Zipper interface {
	// ConfigWorkflow will register workflows from config files to zipper.
	ConfigWorkflow(conf string) error
	// ListenAndServe start zipper as server.
	ListenAndServe() error
	// ReadConfigFile(conf string) error
	// AddWorkflow(wf ...core.Workflow) error
	// ConfigDownstream(opts ...interface{}) error
	// Connect() error
	AddDownstreamZipper(downstream Zipper) error
	// RemoveDownstreamZipper(downstream Zipper) error
	// ListenAddr() string
	Addr() string
	Stats() int
	Close() error
}

// zipper is the implementation of Zipper interface.
type zipper struct {
	token             string
	addr              string
	listenAddr        string
	hasDownstreams    bool
	server            *core.Server
	client            *core.Client
	downstreamZippers []Zipper
	// logger            utils.Logger
}

var _ Zipper = &zipper{}

// NewDownstreamZipper create a zipper descriptor for downstream zipper.
func NewDownstreamZipper(name string, opts ...Option) Zipper {
	options := newOptions(opts...)
	client := core.NewClient(name, core.ConnTypeUpstreamZipper)

	return &zipper{
		token:      name,
		listenAddr: options.ZipperListenAddr,
		addr:       options.ZipperAddr,
		client:     client,
	}
}

// NewZipperWithOptions create a zipper instance.
func NewZipperWithOptions(name string, opts ...Option) Zipper {
	options := newOptions(opts...)
	return createZipperServer(name, options.ZipperListenAddr)
}

// NewZipper create a zipper instance from config files.
func NewZipper(conf string) (Zipper, error) {
	config, err := util.ParseConfig(conf)
	if err != nil {
		return nil, err
	}
	// listening address
	listenAddr := fmt.Sprintf("%s:%d", config.Host, config.Port)

	return createZipperServer(config.Name, listenAddr), nil
}

/*************** Server ONLY ***************/
// createZipperServer create a zipper instance as server.
func createZipperServer(name string, addr string) *zipper {
	// create underlying QUIC server
	srv := core.NewServer(name)
	z := &zipper{
		server:     srv,
		token:      name,
		listenAddr: addr,
	}
	// initialize
	z.init()
	return z
}

// initialize when zipper running as server. support inspection:
// - `kill -SIGUSR1 <pid>` inspect state()
// - `kill -SIGTERM <pid>` graceful shutdown
// - `kill -SIGUSR2 <pid>` inspect golang GC
func (z *zipper) init() {
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGUSR2, syscall.SIGUSR1, syscall.SIGINT)
		logger.Printf("Listening signals ...")
		for p1 := range c {
			logger.Printf("Received signal: %s", p1)
			if p1 == syscall.SIGTERM || p1 == syscall.SIGINT {
				logger.Printf("graceful shutting down ... %s", p1)
				os.Exit(0)
				// close(sgnl)
			} else if p1 == syscall.SIGUSR2 {
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				fmt.Printf("\tNumGC = %v\n", m.NumGC)
			} else if p1 == syscall.SIGUSR1 {
				logger.Printf("print zipper stats(): %d", z.Stats())
			}
		}
	}()
}

func (z *zipper) ConfigWorkflow(conf string) error {
	config, err := util.ParseConfig(conf)
	if err != nil {
		return err
	}
	for i, app := range config.Functions {
		if err := z.server.AddWorkflow(core.Workflow{Seq: i, Token: app.Name}); err != nil {
			return err
		}
	}
	return nil
}

// // ReadConfigFile read zipper configs from workflow.yaml file
// func (s *zipper) ReadConfigFile(conf string) error {
// 	config, err := util.ParseConfig(conf)
// 	if err != nil {
// 		return err
// 	}
// 	for i, app := range config.Functions {
// 		if err := s.server.AddWorkflow(core.Workflow{Seq: i, Token: app.Name}); err != nil {
// 			return err
// 		}
// 	}
// 	return nil
// }

// func (s *zipper) AddWorkflow(wfs ...core.Workflow) error {
// 	return s.server.AddWorkflow(wfs...)
// }

// // ConfigDownstream will add a downstream zipper to upstream zipper.
// func (s *zipper) ConfigDownstream(opts ...interface{}) error {
// 	return nil
// }

// ListenAndServe will start zipper service.
func (s *zipper) ListenAndServe() error {
	logger.Debugf("%sCreating Zipper Server ...", zipperLogPrefix)
	return s.server.ListenAndServe(context.Background(), s.listenAddr)
}

/*************** Client ONLY ****************************
/* 在 Zipper 级联场景中，Zipper 本身也可以作为 QUIC Client，
/* 此时，我们称该种 Zipper 为 Upstream Zipper。
*********************************************************/

// Client：建立到 downstream zipper 的连接
func (s *zipper) Connect() error {
	err := s.client.Connect(context.Background(), s.addr)
	if err != nil {
		logger.Errorf("%sConnect() error: %s", zipperLogPrefix, err)
		return err
	}

	return nil
}

// AddDownstreamZipper will add downstream zipper.
func (s *zipper) AddDownstreamZipper(downstream Zipper) error {
	logger.Debugf("%sAddDownstreamZipper: %v", zipperLogPrefix, downstream)
	s.downstreamZippers = append(s.downstreamZippers, downstream)
	s.hasDownstreams = true
	logger.Debugf("%scurrent downstreams: %v", zipperLogPrefix, s.downstreamZippers)
	return nil
}

// Client：去除 downstream Zipper
func (s *zipper) RemoveDownstreamZipper(downstream Zipper) error {
	index := -1
	for i, v := range s.downstreamZippers {
		if v.Addr() == downstream.Addr() {
			index = i
			break
		}
	}

	// remove from slice
	s.downstreamZippers = append(s.downstreamZippers[:index], s.downstreamZippers[index+1:]...)
	return nil
}

// 获取 zipper 监听的 endpoint
func (s *zipper) ListenAddr() string {
	return s.listenAddr
}

func (s *zipper) Addr() string {
	return s.addr
}

/*************** Client/Server 都用 ***************/

// Client：关闭本地连接；
// Server：关闭服务器；
func (s *zipper) Close() error {
	if s.server != nil {
		if err := s.server.Close(); err != nil {
			logger.Errorf("%s Close(): %v", zipperLogPrefix, err)
			return err
		}
	}
	if s.client != nil {
		if err := s.client.Close(); err != nil {
			logger.Errorf("%s Close(): %v", zipperLogPrefix, err)
			return err
		}
	}
	return nil
}

func (s *zipper) Stats() int {
	logger.Debugf("%sall sfn connected: %d", zipperLogPrefix, len(s.server.StatsFunctions()))
	for k, v := range s.server.StatsFunctions() {
		ids := make([]int64, 0)
		for _, c := range v {
			ids = append(ids, int64((*c).StreamID()))
		}
		logger.Debugf("%s%s-> k=%v, v.StreamID=%v", zipperLogPrefix, k, ids)
	}

	logger.Debugf("%stotal DataFrames received: %d", zipperLogPrefix, s.server.StatsCounter())
	return len(s.server.StatsFunctions())
}
