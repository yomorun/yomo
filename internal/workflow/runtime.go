package workflow

import (
	"context"
	"fmt"
	"io"
	"log"

	"github.com/yomorun/yomo/internal/conf"
	"github.com/yomorun/yomo/pkg/quic"
)

var Clients map[string]Client

type Client struct {
	App        conf.App
	StreamMap  map[int64]Stream
	QuicClient quic.Client
}

type Stream struct {
	St         io.ReadWriter
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
func Build(wfConf *conf.WorkflowConfig, id int64) ([]func() (io.ReadWriter, func()), []func() (io.Writer, func())) {
	//init workflow
	flows := make([]func() (io.ReadWriter, func()), 0)
	sinks := make([]func() (io.Writer, func()), 0)

	for _, app := range wfConf.Flows {
		flows = append(flows, createReadWriter(app, id))
	}

	for _, app := range wfConf.Sinks {
		sinks = append(sinks, createWriter(app, 0))
	}

	return flows, sinks
}

func connectToApp(app conf.App) (quic.Client, error) {
	client, err := quic.NewClient(fmt.Sprintf("%s:%d", app.Host, app.Port))
	if err != nil {
		log.Print(getConnectFailedMsg(app), err)
		return nil, err
	}
	log.Printf("✅ Connect to %s successfully.", getAppInfo(app))
	return client, err
}

func createStream(ctx context.Context, client quic.Client) (quic.Stream, error) {
	return client.CreateStream(ctx)
}

func getConnectFailedMsg(app conf.App) string {
	return fmt.Sprintf("❌ Connect to %s failure with err: ",
		getAppInfo(app))
}

func getAppInfo(app conf.App) string {
	return fmt.Sprintf("%s (%s:%d)",
		app.Name,
		app.Host,
		app.Port)
}

func createReadWriter(app conf.App, id int64) func() (io.ReadWriter, func()) {
	f := func() (io.ReadWriter, func()) {
		if len(Clients[app.Name].StreamMap) > 0 && Clients[app.Name].StreamMap[id].St != nil {
			return Clients[app.Name].StreamMap[id].St, Clients[app.Name].StreamMap[id].CancelFunc
		}

		if Clients[app.Name].StreamMap == nil {
			client, err := connectToApp(app)

			if err != nil {
				return nil, nil
			}
			streammap := make(map[int64]Stream)
			Clients[app.Name] = Client{
				App:        app,
				StreamMap:  streammap,
				QuicClient: client,
			}
		}

		ctx, cancel := context.WithCancel(context.Background())
		stream, err := createStream(ctx, Clients[app.Name].QuicClient)
		if err != nil {
			return nil, cancelStream(cancel, app, id)
		}
		Clients[app.Name].StreamMap[id] = Stream{
			St:         stream,
			CancelFunc: cancelStream(cancel, app, id),
		}

		return stream, cancelStream(cancel, app, id)
	}

	return f
}

func createWriter(app conf.App, id int64) func() (io.Writer, func()) {
	f := func() (io.Writer, func()) {
		if len(Clients[app.Name].StreamMap) > 0 && Clients[app.Name].StreamMap[id].St != nil {
			return Clients[app.Name].StreamMap[id].St, Clients[app.Name].StreamMap[id].CancelFunc
		}

		if Clients[app.Name].StreamMap == nil {
			client, err := connectToApp(app)

			if err != nil {
				return nil, nil
			}
			streammap := make(map[int64]Stream)
			Clients[app.Name] = Client{
				App:        app,
				StreamMap:  streammap,
				QuicClient: client,
			}
		}

		ctx, cancel := context.WithCancel(context.Background())
		stream, err := createStream(ctx, Clients[app.Name].QuicClient)
		if err != nil {
			return nil, cancelStream(cancel, app, id)
		}

		Clients[app.Name].StreamMap[id] = Stream{
			St:         stream,
			CancelFunc: cancelStream(cancel, app, id),
		}

		return stream, cancelStream(cancel, app, id)
	}

	return f
}

func cancelStream(cancel context.CancelFunc, app conf.App, id int64) func() {
	f := func() {
		cancel()
		Clients[app.Name].StreamMap[id] = Stream{
			St:         nil,
			CancelFunc: cancelStream(cancel, app, id),
		}
	}
	return f
}
