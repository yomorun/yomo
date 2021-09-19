package yomo

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/yomorun/yomo/internal/core"
	"github.com/yomorun/yomo/internal/util"
	"github.com/yomorun/yomo/pkg/logger"
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

	// AddDownstreamZipper will add downstream zipper.
	AddDownstreamZipper(downstream Zipper) error

	// Addr returns the listen address of zipper.
	Addr() string

	// Stats return insight data
	Stats() int

	// Close will close the zipper.
	Close() error

	// ReadConfigFile(conf string) error
	// AddWorkflow(wf ...core.Workflow) error
	// ConfigDownstream(opts ...interface{}) error
	// Connect() error
	// RemoveDownstreamZipper(downstream Zipper) error
	// ListenAddr() string
}

// zipper is the implementation of Zipper interface.
type zipper struct {
	token             string
	addr              string
	hasDownstreams    bool
	server            *core.Server
	client            *core.Client
	downstreamZippers []Zipper
	// logger            utils.Logger
}

var _ Zipper = &zipper{}

// NewZipperWithOptions create a zipper instance.
func NewZipperWithOptions(name string, opts ...Option) Zipper {
	options := newOptions(opts...)
	return createZipperServer(name, options.ZipperAddr)
}

// NewZipper create a zipper instance from config files.
func NewZipper(conf string) (Zipper, error) {
	config, err := util.ParseConfig(conf)
	if err != nil {
		logger.Errorf("%s[ERR] %v", zipperLogPrefix, err)
		return nil, err
	}
	// listening address
	listenAddr := fmt.Sprintf("%s:%d", config.Host, config.Port)

	return createZipperServer(config.Name, listenAddr), nil
}

// NewDownstreamZipper create a zipper descriptor for downstream zipper.
func NewDownstreamZipper(name string, opts ...Option) Zipper {
	options := newOptions(opts...)
	client := core.NewClient(name, core.ClientTypeUpstreamZipper)

	return &zipper{
		token:  name,
		addr:   options.ZipperAddr,
		client: client,
	}
}

/*************** Server ONLY ***************/
// createZipperServer create a zipper instance as server.
func createZipperServer(name string, addr string) *zipper {
	// create underlying QUIC server
	srv := core.NewServer(name)
	z := &zipper{
		server: srv,
		token:  name,
		addr:   addr,
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
		logger.Printf("%sListening SIGUSR1, SIGUSR2, SIGTERM/SIGINT...", zipperLogPrefix)
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

// ConfigWorkflow will read workflows from config files and register them to zipper.
func (z *zipper) ConfigWorkflow(conf string) error {
	config, err := util.ParseConfig(conf)
	if err != nil {
		return err
	}
	for i, app := range config.Functions {
		if err := z.server.AddWorkflow(core.Workflow{Seq: i, Token: app.Name}); err != nil {
			return err
		}
		logger.Printf("%s[AddWorkflow] %d, %s", zipperLogPrefix, i, app.Name)
	}
	return nil
}

// ListenAndServe will start zipper service.
func (z *zipper) ListenAndServe() error {
	logger.Debugf("%sCreating Zipper Server ...", zipperLogPrefix)
	// check downstream zippers
	for _, ds := range z.downstreamZippers {
		if dsZipper, ok := ds.(*zipper); ok {
			dsZipper.client.Connect(context.Background(), dsZipper.addr)
			z.server.AddDownstreamServer(dsZipper.addr, dsZipper.client)
		}
	}
	return z.server.ListenAndServe(context.Background(), z.addr)
}

// AddDownstreamZipper will add downstream zipper.
func (z *zipper) AddDownstreamZipper(downstream Zipper) error {
	logger.Debugf("%sAddDownstreamZipper: %v", zipperLogPrefix, downstream)
	z.downstreamZippers = append(z.downstreamZippers, downstream)
	z.hasDownstreams = true
	logger.Debugf("%scurrent downstreams: %v", zipperLogPrefix, z.downstreamZippers)
	return nil
}

// RemoveDownstreamZipper remove downstream zipper.
func (z *zipper) RemoveDownstreamZipper(downstream Zipper) error {
	index := -1
	for i, v := range z.downstreamZippers {
		if v.Addr() == downstream.Addr() {
			index = i
			break
		}
	}

	// remove from slice
	z.downstreamZippers = append(z.downstreamZippers[:index], z.downstreamZippers[index+1:]...)
	return nil
}

// Addr returns listen address of zipper.
func (z *zipper) Addr() string {
	return z.addr
}

// Close will close a connection. If zipper is Server, close the server. If zipper is Client, close the client.
func (z *zipper) Close() error {
	if z.server != nil {
		if err := z.server.Close(); err != nil {
			logger.Errorf("%s Close(): %v", zipperLogPrefix, err)
			return err
		}
	}
	if z.client != nil {
		if err := z.client.Close(); err != nil {
			logger.Errorf("%s Close(): %v", zipperLogPrefix, err)
			return err
		}
	}
	return nil
}

// Stats inspects current server.
func (z *zipper) Stats() int {
	log.Printf("[%s] all sfn connected: %d", z.token, len(z.server.StatsFunctions()))
	for k, v := range z.server.StatsFunctions() {
		ids := make([]int64, 0)
		for _, c := range v {
			ids = append(ids, int64((*c).StreamID()))
		}
		log.Printf("[%s] -> k=%v, v.StreamID=%v", z.token, k, ids)
	}

	log.Printf("[%s] all downstream zippers connected: %d", z.token, len(z.server.Downstreams()))
	for k, v := range z.server.Downstreams() {
		log.Printf("[%s] |> [%s] %s", z.token, k, v.ServerAddr())
	}

	log.Printf("[%s] total DataFrames received: %d", z.token, z.server.StatsCounter())

	return len(z.server.StatsFunctions())
}
