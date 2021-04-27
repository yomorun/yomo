package workflow

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"

	"github.com/yomorun/yomo/internal/conf"
	"github.com/yomorun/yomo/pkg/quic"
)

var FlowClients map[string]Client
var SinkClients map[string]Client
var flowmutex sync.RWMutex
var sinkmutex sync.RWMutex

type Client struct {
	App        conf.App
	StreamMap  map[int64]Stream
	QuicClient quic.Client
}

type Stream struct {
	St         quic.Stream
	CancelFunc context.CancelFunc
}

func init() {
	FlowClients = make(map[string]Client)
	SinkClients = make(map[string]Client)
}

// Run runs quic service
func Run(endpoint string, handle quic.ServerHandler) error {
	server := quic.NewServer(handle)

	return server.ListenAndServe(context.Background(), endpoint)
}

// Build build the workflow by config (.yaml).
func Build(wfConf *conf.WorkflowConfig, id int64) ([]func() (quic.Stream, func()), []func() (io.Writer, func())) {
	//init workflow
	flows := make([]func() (quic.Stream, func()), 0)
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

func createReadWriter(app conf.App, id int64) func() (quic.Stream, func()) {
	f := func() (quic.Stream, func()) {
		flowmutex.Lock()
		if len(FlowClients[app.Name].StreamMap) > 0 && FlowClients[app.Name].StreamMap[id].St != nil {
			flowmutex.Unlock()
			return FlowClients[app.Name].StreamMap[id].St, FlowClients[app.Name].StreamMap[id].CancelFunc
		}

		if FlowClients[app.Name].StreamMap == nil || (FlowClients[app.Name].StreamMap != nil && FlowClients[app.Name].QuicClient == nil) {
			client, err := connectToApp(app)

			if err != nil {
				flowmutex.Unlock()
				return nil, nil
			}
			streammap := make(map[int64]Stream)
			FlowClients[app.Name] = Client{
				App:        app,
				StreamMap:  streammap,
				QuicClient: client,
			}
		}

		ctx, cancel := context.WithCancel(context.Background())
		stream, err := createStream(ctx, FlowClients[app.Name].QuicClient)
		if err != nil {
			log.Print(getConnectFailedMsg(app), err)
			if strings.Contains(err.Error(), "No recent network activity") {
				FlowClients[app.Name] = Client{
					App:        app,
					StreamMap:  nil,
					QuicClient: nil,
				}
			}
			flowmutex.Unlock()
			return nil, cancelFlowStream(cancel, app, id)
		}
		FlowClients[app.Name].StreamMap[id] = Stream{
			St:         stream,
			CancelFunc: cancelFlowStream(cancel, app, id),
		}
		flowmutex.Unlock()
		return stream, cancelFlowStream(cancel, app, id)
	}

	return f
}

func createWriter(app conf.App, id int64) func() (io.Writer, func()) {

	f := func() (io.Writer, func()) {
		sinkmutex.Lock()
		if len(SinkClients[app.Name].StreamMap) > 0 && SinkClients[app.Name].StreamMap[id].St != nil {
			sinkmutex.Unlock()
			return SinkClients[app.Name].StreamMap[id].St, SinkClients[app.Name].StreamMap[id].CancelFunc
		}

		if SinkClients[app.Name].StreamMap == nil || (SinkClients[app.Name].StreamMap != nil && SinkClients[app.Name].QuicClient == nil) {
			client, err := connectToApp(app)

			if err != nil {
				sinkmutex.Unlock()
				return nil, nil
			}

			streammap := make(map[int64]Stream)
			SinkClients[app.Name] = Client{
				App:        app,
				StreamMap:  streammap,
				QuicClient: client,
			}

		}

		ctx, cancel := context.WithCancel(context.Background())
		stream, err := createStream(ctx, SinkClients[app.Name].QuicClient)
		if err != nil {
			log.Print(getConnectFailedMsg(app), err)
			if strings.Contains(err.Error(), "No recent network activity") {
				SinkClients[app.Name] = Client{
					App:        app,
					StreamMap:  nil,
					QuicClient: nil,
				}
			}
			sinkmutex.Unlock()
			return nil, cancelSinkStream(cancel, app, id)
		}
		SinkClients[app.Name].StreamMap[id] = Stream{
			St:         stream,
			CancelFunc: cancelSinkStream(cancel, app, id),
		}
		sinkmutex.Unlock()
		return stream, cancelSinkStream(cancel, app, id)
	}

	return f
}

func cancelFlowStream(cancel context.CancelFunc, app conf.App, id int64) func() {
	f := func() {
		flowmutex.Lock()
		if FlowClients[app.Name].StreamMap != nil {
			stream := FlowClients[app.Name].StreamMap[id]
			stream.St = nil
			FlowClients[app.Name].StreamMap[id] = stream
		}
		flowmutex.Unlock()
	}
	return f
}

func cancelSinkStream(cancel context.CancelFunc, app conf.App, id int64) func() {
	f := func() {
		sinkmutex.Lock()
		if SinkClients[app.Name].StreamMap != nil {
			stream := SinkClients[app.Name].StreamMap[id]
			stream.St = nil
			SinkClients[app.Name].StreamMap[id] = stream
		}
		sinkmutex.Unlock()
	}
	return f
}
