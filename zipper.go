package yomo

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/core/router"
	"github.com/yomorun/yomo/core/ylog"
	"github.com/yomorun/yomo/pkg/config"
	"golang.org/x/exp/slog"
)

// Zipper is the orchestrator of yomo. There are two types of zipper:
// one is Upstream Zipper, which is used to connect to multiple downstream zippers,
// another one is Downstream Zipper (will call it as Zipper directly), which is used
// to connected by `Upstream Zipper`, `Source` and `Stream Function`
type Zipper interface {
	// ConfigWorkflow will register workflows from config files to zipper.
	ConfigWorkflow(conf string) error

	// ConfigMesh will register edge-mesh config URL
	ConfigMesh(url string) error

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

	// InitOptions initialize options
	InitOptions(opts ...Option)

	// ReadConfigFile(conf string) error
	// AddWorkflow(wf ...core.Workflow) error
	// ConfigDownstream(opts ...interface{}) error
	// Connect() error
	// RemoveDownstreamZipper(downstream Zipper) error
	// ListenAddr() string
}

// zipper is the implementation of Zipper interface.
type zipper struct {
	name              string
	addr              string
	hasDownstreams    bool
	server            *core.Server
	client            *core.Client
	downstreamZippers []Zipper
	wfc               *config.WorkflowConfig
}

var _ Zipper = &zipper{}

// NewZipperWithOptions create a zipper instance.
func NewZipperWithOptions(name string, opts ...Option) Zipper {
	options := NewOptions(opts...)
	zipper := createZipperServer(name, options, nil)
	zipper.ConfigMesh(options.MeshConfigURL)

	return zipper
}

// NewZipper create a zipper instance from config files.
func NewZipper(conf string) (Zipper, error) {
	config, err := config.ParseWorkflowConfig(conf)
	if err != nil {
		return nil, err
	}
	// listening address
	listenAddr := fmt.Sprintf("%s:%d", config.Host, config.Port)

	options := NewOptions()
	options.ZipperAddr = listenAddr
	zipper := createZipperServer(config.Name, options, config)
	// zipper workflow
	err = zipper.configWorkflow(config)
	zipper.server.Logger().Info("Using config file", "file_path", conf)

	return zipper, err
}

// NewDownstreamZipper create a zipper descriptor for downstream zipper.
func NewDownstreamZipper(name string, opts ...Option) Zipper {
	options := NewOptions(opts...)
	client := core.NewClient(name, core.ClientTypeUpstreamZipper, options.ClientOptions...)

	return &zipper{
		name:   name,
		addr:   options.ZipperAddr,
		client: client,
	}
}

/*************** Server ONLY ***************/
// createZipperServer create a zipper instance as server.
func createZipperServer(name string, options *Options, cfg *config.WorkflowConfig) *zipper {
	// create underlying QUIC server
	srv := core.NewServer(name, options.ServerOptions...)
	z := &zipper{
		server: srv,
		name:   name,
		addr:   options.ZipperAddr,
		wfc:    cfg,
	}
	// initialize
	z.init()
	return z
}

func (z *zipper) Logger() *slog.Logger {
	logger := z.server.Logger()
	if logger == nil {
		return ylog.Default()
	}

	return logger
}

// ConfigWorkflow will read workflows from config files and register them to zipper.
func (z *zipper) ConfigWorkflow(conf string) error {
	config, err := config.ParseWorkflowConfig(conf)
	if err != nil {
		return err
	}
	z.Logger().Debug("ConfigWorkflow", "work_flow", config)
	return z.configWorkflow(config)
}

func (z *zipper) configWorkflow(config *config.WorkflowConfig) error {
	z.wfc = config
	z.server.ConfigMetadataDecoder(metadata.DefaultDecoder())
	z.server.ConfigRouter(router.Default(config.Functions))
	return nil
}

func (z *zipper) ConfigMesh(url string) error {
	if url == "" {
		return nil
	}

	z.Logger().Debug("Downloading mesh config...")
	// download mesh conf
	res, err := http.Get(url)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	decoder := json.NewDecoder(res.Body)
	var configs []config.MeshZipper
	err = decoder.Decode(&configs)
	if err != nil {
		z.Logger().Error("download Mesh config", err)
		return err
	}
	z.Logger().Debug("download Mesh config successfully")

	if len(configs) == 0 {
		return nil
	}

	for _, downstream := range configs {
		if downstream.Name == z.name {
			continue
		}
		addr := fmt.Sprintf("%s:%d", downstream.Host, downstream.Port)
		opts := []Option{WithZipperAddr(addr)}
		if downstream.Credential != "" {
			opts = append(opts, WithCredential(downstream.Credential))
		}
		z.AddDownstreamZipper(NewDownstreamZipper(downstream.Name, opts...))
	}

	return nil
}

// ListenAndServe will start zipper service.
func (z *zipper) ListenAndServe() error {
	z.Logger().Debug("Creating Zipper Server")
	// check downstream zippers
	for _, ds := range z.downstreamZippers {
		if dsZipper, ok := ds.(*zipper); ok {
			go func(dsZipper *zipper) {
				dsZipper.client.Connect(context.Background(), dsZipper.addr)
				z.server.AddDownstreamServer(dsZipper.addr, dsZipper.client)
			}(dsZipper)
		}
	}
	return z.server.ListenAndServe(context.Background(), z.addr)
}

// AddDownstreamZipper will add downstream zipper.
func (z *zipper) AddDownstreamZipper(downstream Zipper) error {
	z.downstreamZippers = append(z.downstreamZippers, downstream)
	z.hasDownstreams = true
	z.Logger().Debug("add downstreams", "zipper", downstream.Addr(), "add_num_after", len(z.downstreamZippers))
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
			return err
		}
	}
	if z.client != nil {
		if err := z.client.Close(); err != nil {
			return err
		}
	}
	return nil
}

// Stats inspects current server.
func (z *zipper) Stats() int {
	log.Printf("[%s] all connections: %d", z.name, len(z.server.StatsFunctions()))
	for connID, name := range z.server.StatsFunctions() {
		log.Printf("[%s] -> ConnID=%s, Name=%s", z.name, connID, name)
	}

	log.Printf("[%s] all downstream zippers connected: %d", z.name, len(z.server.Downstreams()))
	for k, v := range z.server.Downstreams() {
		log.Printf("[%s] |> [%s] %v", z.name, k, v)
	}

	log.Printf("[%s] total DataFrames received: %d", z.name, z.server.StatsCounter())

	return len(z.server.StatsFunctions())
}

func (z *zipper) InitOptions(opts ...Option) {
	options := &Options{ZipperAddr: z.addr}
	for _, o := range opts {
		o(options)
	}
	srv := core.NewServer(z.name, options.ServerOptions...)
	z.server = srv
	if z.wfc != nil {
		z.configWorkflow(z.wfc)
	}
}
