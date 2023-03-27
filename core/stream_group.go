package core

import (
	"context"
	"sync"

	"golang.org/x/exp/slog"
)

// StreamGroup is the group of stream includes ControlStream amd DataStream.
// One Connection has many DataStream and only one ControlStream, ControlStream authenticates
// Connection and recevies HandshakeFrame and CloseStreamFrame to create DataStream or close
// stream. the ControlStream always the first stream established between server and client.
type StreamGroup struct {
	ctx           context.Context
	controlStream ServerControlStream
	connector     *Connector
	logger        *slog.Logger
	group         sync.WaitGroup
}

// NewStreamGroup returns StreamGroup.
func NewStreamGroup(ctx context.Context, controlStream ServerControlStream, connector *Connector, logger *slog.Logger) *StreamGroup {
	group := &StreamGroup{
		ctx:           ctx,
		controlStream: controlStream,
		connector:     connector,
		logger:        logger,
	}
	return group
}

// Run run contextFunc with connector.
// Run continus Accepts DataStream and create a Context to run with contextFunc.
// TODO: run in aop model, like setMetadata -> handleRoute -> before -> handle -> after.
func (g *StreamGroup) Run(contextFunc func(c *Context)) error {
	for {
		dataStream, err := g.controlStream.AcceptStream(g.ctx)
		if err != nil {
			return err
		}

		g.group.Add(1)
		g.connector.Add(dataStream.ID(), dataStream)

		go func() {
			defer func() {
				g.group.Done()
				g.connector.Remove(dataStream.ID())
			}()

			c := newContext(dataStream, g.logger)
			defer c.Clean()

			contextFunc(c)
		}()
	}
}

// Wait waits all dataStream down.
func (g *StreamGroup) Wait() { g.group.Wait() }
