package core

import (
	"context"
	"errors"
	"sync"

	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/core/router"
	"github.com/yomorun/yomo/core/yerr"
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
	mb            metadata.Builder
	router        router.Router
	logger        *slog.Logger
	group         sync.WaitGroup
}

// NewStreamGroup returns StreamGroup.
func NewStreamGroup(
	ctx context.Context,
	controlStream ServerControlStream,
	connector *Connector,
	mb metadata.Builder,
	router router.Router,
	logger *slog.Logger,
) *StreamGroup {
	group := &StreamGroup{
		ctx:           ctx,
		controlStream: controlStream,
		connector:     connector,
		mb:            mb,
		router:        router,
		logger:        logger,
	}
	return group
}

func (g *StreamGroup) handleRoute(hf *frame.HandshakeFrame, md metadata.Metadata) (router.Route, error) {
	if hf.StreamType() != byte(StreamTypeStreamFunction) {
		return nil, nil
	}
	// route for sfn.
	route := g.router.Route(md)
	if route == nil {
		return nil, errors.New("yomo: can't find route in handshake metadata")
	}
	err := route.Add(hf.ID(), hf.Name(), hf.ObserveDataTags())
	if err == nil {
		return route, nil
	}
	// If there is a stream with the same name as the new stream, replace the old stream with the new one.
	if e := new(yerr.DuplicateNameError); errors.As(err, e) {
		existsStreamID := e.StreamID()
		stream, ok, err := g.connector.Get(existsStreamID)
		if err != nil {
			return nil, err
		}
		if ok {
			stream.Close()
			g.connector.Remove(existsStreamID)
		}
	}
	return route, nil
}

type handshakeResult struct {
	route router.Route
	md    metadata.Metadata
}

// makeHandshakeFunc creates a function that will handle a HandshakeFrame.
// It takes metadata and route parameters, which will be assigned after the returned function is executed.
func (g *StreamGroup) makeHandshakeFunc(result *handshakeResult) func(hf *frame.HandshakeFrame) error {
	return func(hf *frame.HandshakeFrame) (err error) {
		_, ok, err := g.connector.Get(hf.ID())
		if err != nil {
			return
		}
		if ok {
			return errors.New("yomo: stream id is not allowed to be a duplicate")
		}
		md, err := g.mb.Decode(hf.Metadata())
		if err != nil {
			return
		}
		result.md = md

		route, err := g.handleRoute(hf, md)
		if err != nil {
			return
		}
		result.route = route

		return nil
	}
}

// Run run contextFunc with connector.
// Run continuous Accepts DataStream and create a Context to run with contextFunc.
// TODO: run in aop model, like before -> handle -> after.
func (g *StreamGroup) Run(contextFunc func(c *Context)) error {
	for {
		var routeResult handshakeResult

		handshakeFunc := g.makeHandshakeFunc(&routeResult)

		dataStream, err := g.controlStream.OpenStream(g.ctx, handshakeFunc)
		if err != nil {
			return err
		}

		g.group.Add(1)
		g.connector.Add(dataStream.ID(), dataStream)

		go g.handleContextFunc(routeResult.md, routeResult.route, dataStream, contextFunc)
	}
}

func (g *StreamGroup) handleContextFunc(mb metadata.Metadata, route router.Route, dataStream DataStream, contextFunc func(c *Context)) {
	defer func() {
		// source route is always nil.
		if route != nil {
			route.Remove(dataStream.ID())
		}
		g.connector.Remove(dataStream.ID())
		g.group.Done()
	}()

	c := newContext(dataStream, mb, route, g.logger)
	defer c.Clean()

	contextFunc(c)
}

// Wait waits all dataStream down.
func (g *StreamGroup) Wait() { g.group.Wait() }
