package workflow

import (
	"context"
	"fmt"
	"io"

	"github.com/yomorun/yomo/internal/conf"
	"github.com/yomorun/yomo/pkg/quic"
)

var Clients map[string]Client

type Client struct {
	App        conf.App
	Stream     io.ReadWriter
	CancelFunc context.CancelFunc
}

func init() {
	Clients = make(map[string]Client)
}

// Run runs quic service
func Run(endpoint string, handle quic.ServerHandler) error {
	server := quic.NewServer(handle)

	return server.ListenAndServe(context.Background(), endpoint)
}

// Build build the workflow by config (.yaml).
func Build(wfConf *conf.WorkflowConfig) []func() (io.ReadWriter, func()) {
	//init workflow
	flows := make([]func() (io.ReadWriter, func()), 0)

	for _, app := range wfConf.Flows {
		flows = append(flows, createReadWriter(app))
	}

	for _, app := range wfConf.Sinks {
		flows = append(flows, createReadWriter(app))
	}

	return flows
}

func connectToApp(app conf.App, ctx context.Context) (quic.Stream, error) {
	client, err := quic.NewClient(fmt.Sprintf("%s:%d", app.Host, app.Port))
	if err != nil {
		return nil, err
	}
	return client.CreateStream(ctx)
}

func getConnectFailedMsg(appType string, app conf.App) string {
	return fmt.Sprintf("❌ Connect to %s failure with err: ",
		getAppInfo(appType, app))
}

func getAppInfo(appType string, app conf.App) string {
	return fmt.Sprintf("%s %s (%s:%d)",
		appType,
		app.Name,
		app.Host,
		app.Port)
}

func createReadWriter(app conf.App) func() (io.ReadWriter, func()) {
	f := func() (io.ReadWriter, func()) {
		if Clients[app.Name].Stream != nil {
			return Clients[app.Name].Stream, Clients[app.Name].CancelFunc
		} else {
			ctx, cancel := context.WithCancel(context.Background())
			stream, err := connectToApp(app, ctx)
			if err != nil {
				Clients[app.Name] = Client{
					App:        app,
					Stream:     nil,
					CancelFunc: cancelStream(cancel, app),
				}
				return nil, cancelStream(cancel, app)
			} else {
				Clients[app.Name] = Client{
					App:        app,
					Stream:     stream,
					CancelFunc: cancelStream(cancel, app),
				}

				return stream, cancelStream(cancel, app)
			}
		}

	}

	return f
}

func cancelStream(cancel context.CancelFunc, app conf.App) func() {
	f := func() {
		cancel()
		Clients[app.Name] = Client{
			App:    app,
			Stream: nil,
		}
	}
	return f
}
