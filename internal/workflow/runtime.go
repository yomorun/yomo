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
	App    conf.App
	Stream io.ReadWriter
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
func Build(wfConf *conf.WorkflowConfig) []func() io.ReadWriter {
	//init workflow
	actions := make([]func() io.ReadWriter, 0)

	for _, app := range wfConf.Actions {
		f := func() io.ReadWriter {

			if Clients[app.Name].Stream != nil {
				return Clients[app.Name].Stream
			} else {
				stream, err := connectToApp(app)
				if err != nil {
					Clients[app.Name] = Client{
						App:    app,
						Stream: nil,
					}
					return nil
				} else {
					Clients[app.Name] = Client{
						App:    app,
						Stream: stream,
					}
					return stream
				}
			}

		}
		actions = append(actions, f)

	}

	for _, app := range wfConf.Sinks {
		f := func() io.ReadWriter {
			if Clients[app.Name].Stream != nil {
				return Clients[app.Name].Stream
			} else {
				stream, err := connectToApp(app)
				if err != nil {
					Clients[app.Name] = Client{
						App:    app,
						Stream: nil,
					}
					return nil
				} else {
					Clients[app.Name] = Client{
						App:    app,
						Stream: stream,
					}
					return stream
				}
			}
		}

		actions = append(actions, f)
	}

	return actions
}

func connectToApp(app conf.App) (quic.Stream, error) {
	client, err := quic.NewClient(fmt.Sprintf("%s:%d", app.Host, app.Port))
	if err != nil {
		return nil, err
	}

	return client.CreateStream(context.Background())
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
