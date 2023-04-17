package yomo

import (
	"context"
	"fmt"

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

// RunZipper run a zipper from a config file.
func RunZipper(ctx context.Context, configPath string) error {
	conf, err := config.ParseConfigFile(configPath)
	if err != nil {
		return err
	}

	// listening address.
	listenAddr := fmt.Sprintf("%s:%d", conf.Host, conf.Port)

	//
	serverOptions := []core.ServerOption{}
	if conf.Auth.Type == "token" {
		serverOptions = append(serverOptions, WithAuth("token", conf.Auth.Token))
	}

	zipper, err := NewZipper(conf.Name, conf.Functions, conf.Downstreams, WithDownstreamOption(serverOptions...))
	if err != nil {
		return err
	}
	zipper.Logger().Info("using config file", "file_path", conf)

	return zipper.ListenAndServe(ctx, listenAddr)
}

// NewZipper returns a zipper.
func NewZipper(name string, functions []config.Function, meshConfig []config.Downstream, options ...ZipperOption) (Zipper, error) {
	opts := &zipperOptions{}

	for _, o := range options {
		o(opts)
	}

	server := core.NewServer(name, opts.downstreamZipperOption...)

	// add downstreams to server.
	downstreamMap := make(map[string]*core.Client)
	for _, meshConf := range meshConfig {
		addr := fmt.Sprintf("%s:%d", meshConf.Host, meshConf.Port)
		downstreamMap[addr] = core.NewClient(
			meshConf.Name,
			core.ClientTypeUpstreamZipper,
			core.WithCredential(meshConf.Credential),
			core.WithNonBlockWrite(),
			core.WithConnectUntilSucceed(),
		)
	}
	for addr, downstream := range downstreamMap {
		server.Logger().Debug("add downstream", "downstream_addr", addr, "downstream_name", downstream.Name())
		server.AddDownstreamServer(addr, downstream)
	}

	server.ConfigMetadataDecoder(metadata.DefaultDecoder())
	server.ConfigRouter(router.Default(functions))

	// watch signal.
	go waitSignalForShotdownServer(server)

	return server, nil
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
