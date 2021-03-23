package workflow

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/yomorun/yomo/internal/conf"
	"github.com/yomorun/yomo/pkg/quic"
)

var FLOW = []byte{0, 0}
var SINK = []byte{0, 1}

type QuicConn struct {
	Session    quic.Session
	Signal     quic.Stream
	Receivable bool
	Stream     []quic.Stream
	StreamType string
	Name       string
	Heartbeat  chan byte
	IsClose    bool
}

func (c *QuicConn) SendSignal(b []byte) {
	if c.Receivable {
		fmt.Println("server sendddddd", b)
		c.Signal.Write(b)
	}
}
func (c *QuicConn) Init() {
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
				index++
				c.Signal.Write([]byte{1})
				//	c.Beat()
				length = 0
			case 1:
				switch length {
				case 1:
					c.Heartbeat <- value[0]
				case 2:
					if value[0] == byte(0) && value[1] == byte(0) {
						fmt.Println("flow.............................")
						c.Receivable = true
						c.StreamType = "flow"
						c.SendSignal(FLOW)
					}

					if value[0] == byte(0) && value[1] == byte(1) {
						fmt.Println("sink.............................")
						c.Receivable = true
						c.StreamType = "sink"
						c.SendSignal(SINK)
					}
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

			case <-time.After(5 * time.Second):
				c.Close()
			}
		}
	}()

	go func() {
		for {
			time.Sleep(time.Second)
			c.Signal.Write([]byte{0})
		}
	}()
}

func (c *QuicConn) Close() {
	c.Session.CloseWithError(0, "")
	c.IsClose = true
}

// Run runs quic service
func Run(endpoint string, handle quic.ServerHandler) error {
	server := quic.NewServer(handle)

	return server.ListenAndServe(context.Background(), endpoint)
}

// Build build the workflow by config (.yaml).
func Build(wfConf *conf.WorkflowConfig, connMap *map[int64]*QuicConn, last *chan bool) ([]func() (io.ReadWriter, func()), []func() (io.Writer, func())) {
	//init workflow
	flows := make([]func() (io.ReadWriter, func()), 0)
	sinks := make([]func() (io.Writer, func()), 0)

	for {
		select {
		case _, ok := <-*last:
			if !ok {
				return flows, sinks
			}

			for _, app := range wfConf.Flows {
				var conn *QuicConn = nil

				for _, c := range *connMap {
					if c.Name == app.Name {
						conn = c
					}
				}

				flows = append(flows, createReadWriter(conn))
			}

			for _, app := range wfConf.Sinks {
				var conn *QuicConn = nil

				for _, c := range *connMap {
					if c.Name == app.Name {
						conn = c
					}
				}

				sinks = append(sinks, createWriter(conn))
			}

			return flows, sinks
		}
	}

}

func createReadWriter(conn *QuicConn) func() (io.ReadWriter, func()) {
	f := func() (io.ReadWriter, func()) {
		if conn == nil {
			return nil, func() {}
		} else if len(conn.Stream) == 0 {
			return nil, func() {}
		} else {
			last := conn.Stream[len(conn.Stream)-1]
			return last, cancelStream(conn)
		}
	}

	return f
}

func createWriter(conn *QuicConn) func() (io.Writer, func()) {

	f := func() (io.Writer, func()) {
		if conn == nil {
			return nil, func() {}
		} else if len(conn.Stream) == 0 {
			return nil, func() {}
		} else {
			last := conn.Stream[len(conn.Stream)-1]
			return last, cancelStream(conn)
		}
	}
	return f
}

func cancelStream(conn *QuicConn) func() {
	f := func() {
		conn.Close()
	}
	return f
}
