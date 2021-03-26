package workflow

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/yomorun/yomo/internal/conf"
	"github.com/yomorun/yomo/pkg/quic"
)

var GlobalApp = ""

type QuicConn struct {
	Session    quic.Session
	Signal     quic.Stream
	Stream     []io.ReadWriter
	StreamType string
	Name       string
	Heartbeat  chan byte
	IsClose    bool
	Ready      bool
}

func (c *QuicConn) SendSignal(b []byte) {
	c.Signal.Write(b)
}
func (c *QuicConn) Init(conf *conf.WorkflowConfig) {
	index := 0
	go func() {
		for {
			buf := make([]byte, 1024)
			n, err := c.Signal.Read(buf)

			if err != nil {
				break
			}
			value := buf[:n]
			length := len(value)
			switch index {
			case 0:
				c.Name = string(value)
				c.StreamType = "source"
				for _, app := range conf.Flows {
					if app.Name == c.Name {
						c.StreamType = "flow"
					}
				}
				for _, app := range conf.Sinks {
					if app.Name == c.Name {
						c.StreamType = "sink"
					}
				}
				index++
				fmt.Println("Receive App:", c.Name, c.StreamType)
				if c.StreamType == "source" {
					c.Signal.Write([]byte{1})
				} else {
					c.Signal.Write([]byte{2})
				}
				c.Beat()
				length = 0
			case 1:
				switch length {
				case 1:
					c.Heartbeat <- value[0]
				}
			}
		}
	}()
}

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
				c.Close()
			}
		}
	}()

	go func() {
		for {
			time.Sleep(200 * time.Millisecond)
			c.Signal.Write([]byte{0})
		}
	}()
}

func (c *QuicConn) Close() {
	c.Session.CloseWithError(0, "")
	c.IsClose = true
	c.Ready = true
}

// Run runs quic service
func Run(endpoint string, handle quic.ServerHandler) error {
	server := quic.NewServer(handle)

	return server.ListenAndServe(context.Background(), endpoint)
}

// Build build the workflow by config (.yaml).
func Build(wfConf *conf.WorkflowConfig, connMap *map[int64]*QuicConn, index int) ([]func() (io.ReadWriter, func()), []func() (io.Writer, func())) {
	//init workflow

	if GlobalApp == "" {
		for i, v := range wfConf.Sinks {
			if i == 0 {
				GlobalApp = v.Name
			}
		}

		for i, v := range wfConf.Flows {
			if i == 0 {
				GlobalApp = v.Name
			}
		}
	}

	flows := make([]func() (io.ReadWriter, func()), 0)
	sinks := make([]func() (io.Writer, func()), 0)

	for _, app := range wfConf.Flows {
		flows = append(flows, createReadWriter(app, connMap, index))
	}

	for _, app := range wfConf.Sinks {
		sinks = append(sinks, createWriter(app, connMap, index))
	}

	return flows, sinks

}

func createReadWriter(app conf.App, connMap *map[int64]*QuicConn, index int) func() (io.ReadWriter, func()) {
	fmt.Println("flow s.index.:", index)
	f := func() (io.ReadWriter, func()) {
		if app.Name != GlobalApp {
			index = 0
		}

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
		} else if len(conn.Stream) > index && conn.Stream[index] != nil {
			conn.Ready = true
			return conn.Stream[index], cancelStream(app, conn, connMap, id)
		} else {
			if conn.Ready {
				conn.Ready = false
				conn.SendSignal([]byte{0, 0})
			}
			return nil, func() {}
		}

	}

	return f
}

func createWriter(app conf.App, connMap *map[int64]*QuicConn, index int) func() (io.Writer, func()) {
	fmt.Println("sink s.index.:", index)
	f := func() (io.Writer, func()) {
		// if app.Name != GlobalApp {
		// 	index = 0
		// }

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
		} else if len(conn.Stream) > index && conn.Stream[index] != nil {
			conn.Ready = true
			return conn.Stream[index], cancelStream(app, conn, connMap, id)
		} else {
			if conn.Ready {
				conn.Ready = false
				conn.SendSignal([]byte{0, 0})
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
