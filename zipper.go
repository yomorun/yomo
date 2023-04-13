package yomo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/core/router"
	"github.com/yomorun/yomo/pkg/config"
	"golang.org/x/exp/slog"
)

// Zipper is the orchestrator of yomo. There are two types of zipper:
// one is Upstream Zipper, which is used to connect to multiple downstream zippers,
// another one is Downstream Zipper (will call it as Zipper directly), which is used
// to connected by `Upstream Zipper`, `Source` and `Stream Function`.
type Zipper interface {
	// Logger returns the logger of zipper.
	Logger() *slog.Logger

	// ListenAndServe start zipper as server.
	ListenAndServe(context.Context, string) error

	// Close will close the zipper.
	Close() error
}

// RunZipper run a zipper from workflow file and mesh config url.
func RunZipper(ctx context.Context, confPath, meshConfigURL string) error {
	conf, err := config.ParseWorkflowConfig(confPath)
	if err != nil {
		return err
	}
	// listening address
	listenAddr := fmt.Sprintf("%s:%d", conf.Host, conf.Port)

	zipper, err := NewZipper(conf.Name, conf.Workflow.Functions)
	if err != nil {
		return err
	}
	zipper.Logger().Info("using config file", "file_path", conf)

	return zipper.ListenAndServe(ctx, listenAddr)
}

// NewZipper returns a zipper from options.
func NewZipper(name string, functions []config.App, options ...ZipperOption) (Zipper, error) {
	opts := zipperOptions{}
	for _, o := range options {
		o(&opts)
	}

	server := core.NewServer(name, opts.downstreamZipperOption...)

	// add downstreams to server.
	downstreamMap := make(map[string]*core.Client)
	if url := opts.meshConfigURL; url != "" {
		server.Logger().Debug("downdown mesh config successfully", "mesh_config_url", url)
		var err error
		downstreamMap, err = ParseMeshConfig(name, url, opts.UpstreamZipperOption...)
		if err != nil {
			return nil, err
		}
	}
	// meshConfig will cover the configs from meshConfigURL.
	if provider := opts.meshConfigProvider; provider != nil {
		if meshConfig := provider.Provide(); len(meshConfig) != 0 {
			for _, conf := range meshConfig {
				addr := fmt.Sprintf("%s:%d", conf.Host, conf.Port)
				downstreamMap[addr] = core.NewClient(conf.Name, core.ClientTypeUpstreamZipper, core.WithCredential(conf.Credential))
			}
		}
	}

	for addr, downstream := range downstreamMap {
		server.Logger().Debug("mesh config", "downstream_addr", addr, "downstream_name", downstream.Name())
		server.AddDownstreamServer(addr, downstream)
	}

	server.ConfigMetadataDecoder(metadata.DefaultDecoder())
	server.ConfigRouter(router.Default(functions))

	// watch signal.
	go waitSignalForShotdownServer(server)

	return server, nil
}

// ParseMeshConfig
func ParseMeshConfig(omitName, url string, opts ...core.ClientOption) (map[string]*core.Client, error) {
	if url == "" {
		return map[string]*core.Client{}, nil
	}

	// download mesh conf
	res, err := http.Get(url)
	if err != nil {
		return map[string]*core.Client{}, nil
	}
	defer res.Body.Close()

	decoder := json.NewDecoder(res.Body)
	var configs []config.MeshZipper
	err = decoder.Decode(&configs)
	if err != nil {
		return map[string]*core.Client{}, nil
	}

	if len(configs) == 0 {
		return map[string]*core.Client{}, nil
	}

	downstreamMap := make(map[string]*core.Client, len(configs)-1)
	for _, conf := range configs {
		if conf.Name == omitName {
			continue
		}
		addr := fmt.Sprintf("%s:%d", conf.Host, conf.Port)
		opts := []core.ClientOption{}
		if conf.Credential != "" {
			opts = append(opts, WithCredential(conf.Credential))
		}
		downStream := core.NewClient(conf.Name, core.ClientTypeUpstreamZipper, opts...)

		downstreamMap[addr] = downStream
	}

	return downstreamMap, nil
}

func statsToLogger(server *core.Server) {
	logger := server.Logger()

	logger.Info(
		"stats",
		"zipper_name", server.Name(),
		"connector", server.StatsFunctions(),
		"downstreams", server.Downstreams(),
		"data_frame_received_num", server.StatsCounter(),
	)
}
