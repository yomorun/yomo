package workflow

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/yomorun/yomo/internal/conf"
	"github.com/yomorun/yomo/pkg/client"
	"github.com/yomorun/yomo/pkg/quic"
)

const (
	StreamTypeSource string = "source"
	StreamTypeFlow   string = "flow"
	StreamTypeSink   string = "sink"
)

// QuicConn represents the QUIC connection.
type QuicConn struct {
	Session    quic.Session
	Signal     quic.Stream
	Stream     io.ReadWriter
	StreamType string
	Name       string
	Heartbeat  chan byte
	IsClosed   bool
	Ready      bool
}

// SendSignal sends the signal to clients.
func (c *QuicConn) SendSignal(b []byte) error {
	_, err := c.Signal.Write(b)
	return err
}

// Init the QUIC connection.
func (c *QuicConn) Init(conf *conf.WorkflowConfig) {
	isInit := true
	go func() {
		for {
			buf := make([]byte, 1024)
			n, err := c.Signal.Read(buf)

			if err != nil {
				break
			}
			value := buf[:n]

			if isInit {
				// app name
				c.Name = string(value)
				c.StreamType = StreamTypeSource
				// match stream type by name
				for _, app := range conf.Flows {
					if app.Name == c.Name {
						c.StreamType = StreamTypeFlow
						break
					}
				}
				for _, app := range conf.Sinks {
					if app.Name == c.Name {
						c.StreamType = StreamTypeSink
						break
					}
				}
				fmt.Println("Receive App:", c.Name, c.StreamType)
				isInit = false
				c.SendSignal(client.SignalAccepted)
				c.Beat()
				continue
			}

			if bytes.Equal(value, client.SignalHeartbeat) {
				c.Heartbeat <- value[0]
			}
		}
	}()
}

// Beat sends the heartbeat to clients and checks if receiving the heartbeat back.
func (c *QuicConn) Beat() {
	go func() {
		defer c.Close()
		for {
			select {
			case _, ok := <-c.Heartbeat:
				if !ok {
					return
				}

			case <-time.After(time.Second):
				// close the connection if didn't receive the heartbeat after 1s.
				c.Close()
			}
		}
	}()

	go func() {
		for {
			// send heartbeat in every 200ms.
			time.Sleep(200 * time.Millisecond)
			err := c.SendSignal(client.SignalHeartbeat)
			if err != nil {
				break
			}
		}
	}()
}

// Close the QUIC connections.
func (c *QuicConn) Close() {
	c.Session.CloseWithError(0, "")
	c.IsClosed = true
	c.Ready = true
}

// Run QUIC service.
func Run(endpoint string, handle quic.ServerHandler) error {
	server := quic.NewServer(handle)

	return server.ListenAndServe(context.Background(), endpoint)
}

// Build the workflow by config (.yaml).
// It will create one stream for each flows/sinks.
func Build(wfConf *conf.WorkflowConfig, connMap *map[int64]*QuicConn) ([]func() (io.ReadWriter, func()), []func() (io.Writer, func())) {
	//init workflow
	flows := make([]func() (io.ReadWriter, func()), 0)
	sinks := make([]func() (io.Writer, func()), 0)

	for _, app := range wfConf.Flows {
		flows = append(flows, createReadWriter(app, connMap))
	}

	for _, app := range wfConf.Sinks {
		sinks = append(sinks, createWriter(app, connMap))
	}

	return flows, sinks
}

func createReadWriter(app conf.App, connMap *map[int64]*QuicConn) func() (io.ReadWriter, func()) {
	f := func() (io.ReadWriter, func()) {
		var conn *QuicConn = nil
		var id int64 = 0

		for i, c := range *connMap {
			if c.Name == app.Name {
				conn = c
				id = i
			}
		}
		if conn == nil {
			return nil, func() {}
		} else if conn.Stream != nil {
			conn.Ready = true
			return conn.Stream, cancelStream(app, conn, connMap, id)
		} else {
			if conn.Ready {
				conn.Ready = false
				conn.SendSignal(client.SignalFlowSink)
			}
			return nil, func() {}
		}

	}

	return f
}

func createWriter(app conf.App, connMap *map[int64]*QuicConn) func() (io.Writer, func()) {
	f := func() (io.Writer, func()) {
		var conn *QuicConn = nil
		var id int64 = 0

		for i, c := range *connMap {
			if c.Name == app.Name {
				conn = c
				id = i
			}
		}

		if conn == nil {
			return nil, func() {}
		} else if conn.Stream != nil {
			conn.Ready = true
			return conn.Stream, cancelStream(app, conn, connMap, id)
		} else {
			if conn.Ready {
				conn.Ready = false
				conn.SendSignal(client.SignalFlowSink)
			}
			return nil, func() {}
		}

	}
	return f
}

func cancelStream(app conf.App, conn *QuicConn, connMap *map[int64]*QuicConn, id int64) func() {
	f := func() {
		conn.Close()
		delete(*connMap, id)
	}
	return f
}
