package yomo

import (
	"context"
	"fmt"

	"github.com/yomorun/yomo/core"
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

	options := []ZipperOption{}
	if _, ok := conf.Auth["type"]; ok {
		if tokenString, ok := conf.Auth["token"]; ok {
			options = append(options, WithAuth("token", tokenString))
		}
	}

	zipper, err := NewZipper(conf.Name, conf.Downstreams, options...)
	if err != nil {
		return err
	}
	zipper.Logger().Info("using config file", "file_path", configPath)

	return zipper.ListenAndServe(ctx, listenAddr)
}

// NewZipper returns a zipper.
func NewZipper(name string, meshConfig map[string]config.Downstream, options ...ZipperOption) (Zipper, error) {
	opts := &zipperOptions{}

	for _, o := range options {
		o(opts)
	}

	server := core.NewServer(name, opts.serverOption...)

	// add downstreams to server.
	for downstreamName, meshConf := range meshConfig {
		addr := fmt.Sprintf("%s:%d", meshConf.Host, meshConf.Port)

		clientOptions := append(
			opts.clientOption,
			core.WithCredential(meshConf.Credential),
			core.WithNonBlockWrite(),
			core.WithConnectUntilSucceed(),
		)
		downstream := core.NewClient(name, core.ClientTypeUpstreamZipper, clientOptions...)

		server.Logger().Debug("add downstream",
			"downstream_name", downstreamName,
			"downstream_addr", addr,
			"client_id", downstream.ClientID(),
		)
		server.AddDownstreamServer(addr, downstream)
	}

	server.ConfigRouter(router.Default())

	// watch signal.
	go waitSignalForShutdownServer(server)

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
