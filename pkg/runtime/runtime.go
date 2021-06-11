package runtime

import (
	"context"
	"io"
	"sync"

	"github.com/yomorun/yomo/pkg/quic"
	"github.com/yomorun/yomo/pkg/serverless"
)

// Runtime represents the YoMo runtime.
type Runtime interface {
	// Serve a YoMo server.
	Serve(endpoint string) error
}

// NewRuntime inits a new YoMo runtime.
func NewRuntime(conf *WorkflowConfig, meshConfURL string) Runtime {
	return &runtimeImpl{
		conf:        conf,
		meshConfURL: meshConfURL,
	}
}

type runtimeImpl struct {
	conf        *WorkflowConfig
	meshConfURL string
}

// Serve a YoMo server.
func (r *runtimeImpl) Serve(endpoint string) error {
	handler := NewServerHandler(r.conf, r.meshConfURL)
	server := quic.NewServer(handler)

	return server.ListenAndServe(context.Background(), endpoint)
}

// Build the workflow by config (.yaml).
// It will create one stream for each flows/sinks.
func Build(wfConf *WorkflowConfig, connMap *sync.Map) ([]serverless.GetFlowFunc, []serverless.GetSinkFunc) {
	//init workflow
	flows := make([]serverless.GetFlowFunc, 0)
	sinks := make([]serverless.GetSinkFunc, 0)

	for _, app := range wfConf.Flows {
		flows = append(flows, createReadWriter(app, connMap, quic.ConnTypeFlow))
	}

	for _, app := range wfConf.Sinks {
		sinks = append(sinks, createWriter(app, connMap, quic.ConnTypeSink))
	}

	return flows, sinks
}

// GetSinks get sinks from workflow config and connMap
func GetSinks(wfConf *WorkflowConfig, connMap *sync.Map) []serverless.GetSinkFunc {
	sinks := make([]serverless.GetSinkFunc, 0)

	for _, app := range wfConf.Sinks {
		sinks = append(sinks, createWriter(app, connMap, quic.ConnTypeSink))
	}

	return sinks
}

// createReadWriter creates a `GetFlowFunc` for `YoMo-Flow`.
func createReadWriter(app App, connMap *sync.Map, connType string) serverless.GetFlowFunc {
	f := func() (io.ReadWriter, serverless.CancelFunc) {
		id, c := findConn(app, connMap, connType)

		if c == nil {
			return nil, func() {}
		} else if c.conn.Stream != nil {
			return c.conn.Stream, cancelStream(app, c, connMap, id)
		} else {
			c.SendSinkFlowSignal()
			return nil, func() {}
		}
	}

	return f
}

// createWriter creates a `GetSinkFunc` for `YoMo-Sink`.
func createWriter(app App, connMap *sync.Map, connType string) serverless.GetSinkFunc {
	f := func() (io.Writer, serverless.CancelFunc) {
		id, c := findConn(app, connMap, connType)

		if c == nil {
			return nil, func() {}
		} else if c.conn.Stream != nil {
			return c.conn.Stream, cancelStream(app, c, connMap, id)
		} else {
			c.SendSinkFlowSignal()
			return nil, func() {}
		}

	}
	return f
}

func cancelStream(app App, conn *ServerConn, connMap *sync.Map, id int64) func() {
	f := func() {
		conn.Close()
		connMap.Delete(id)
	}
	return f
}

// IsMatched indicates if the connection is matched.
func findConn(app App, connMap *sync.Map, connType string) (int64, *ServerConn) {
	var conn *ServerConn = nil
	var id int64 = 0
	connMap.Range(func(key, value interface{}) bool {
		c := value.(*ServerConn)
		if c.conn.Name == app.Name && c.conn.Type == connType {
			conn = c
			id = key.(int64)

			return false
		}
		return true
	})

	return id, conn
}
