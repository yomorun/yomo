package yomo

import (
	"context"

	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/pkg/id"
)

// Source is responsible for sending data to yomo.
type Source interface {
	// Close will close the connection to YoMo-Zipper.
	Close() error
	// Connect to YoMo-Zipper.
	Connect() error
	// Write the data to directed downstream.
	Write(tag uint32, data []byte) error
	// Broadcast broadcast the data to all downstream.
	Broadcast(tag uint32, data []byte) error
	// SetErrorHandler set the error handler function when server error occurs
	SetErrorHandler(fn func(err error))
	// [Experimental] SetReceiveHandler set the observe handler function
	SetReceiveHandler(fn func(tag uint32, data []byte))
}

// YoMo-Source
type yomoSource struct {
	name       string
	zipperAddr string
	client     *core.Client
	fn         func(uint32, []byte)
}

var _ Source = &yomoSource{}

// NewSource create a yomo-source
func NewSource(name, zipperAddr string, opts ...SourceOption) Source {
	clientOpts := make([]core.ClientOption, len(opts))
	for k, v := range opts {
		clientOpts[k] = core.ClientOption(v)
	}

	client := core.NewClient(name, core.ClientTypeSource, clientOpts...)

	return &yomoSource{
		name:       name,
		zipperAddr: zipperAddr,
		client:     client,
	}
}

// Close will close the connection to YoMo-Zipper.
func (s *yomoSource) Close() error {
	if err := s.client.Close(); err != nil {
		s.client.Logger().Error("failed to close the source", "err", err)
		return err
	}
	s.client.Logger().Debug("the source is closed")
	return nil
}

// Connect to YoMo-Zipper.
func (s *yomoSource) Connect() error {
	// set backflowframe handler
	s.client.SetBackflowFrameObserver(func(frm *frame.BackflowFrame) {
		if s.fn != nil {
			s.fn(frm.Tag, frm.Carriage)
		}
	})

	err := s.client.Connect(context.Background(), s.zipperAddr)
	return err
}

// Write writes data with specified tag.
func (s *yomoSource) Write(tag uint32, data []byte) error {
	md, err := core.NewDefaultMetadata(s.client.ClientID(), false, id.New()).Encode()
	if err != nil {
		return err
	}
	f := &frame.DataFrame{
		Tag:      tag,
		Metadata: md,
		Payload:  data,
	}

	s.client.Logger().Debug("source write", "tag", tag, "data", data)
	return s.client.WriteFrame(f)
}

// SetErrorHandler set the error handler function when server error occurs
func (s *yomoSource) SetErrorHandler(fn func(err error)) {
	s.client.SetErrorHandler(fn)
}

// [Experimental] SetReceiveHandler set the observe handler function
func (s *yomoSource) SetReceiveHandler(fn func(uint32, []byte)) {
	s.fn = fn
	s.client.Logger().Info("source sets receive handler")
}

// Broadcast write the data to all downstreams.
func (s *yomoSource) Broadcast(tag uint32, data []byte) error {
	md, err := core.NewDefaultMetadata(s.client.ClientID(), true, id.New()).Encode()
	if err != nil {
		return err
	}
	f := &frame.DataFrame{
		Tag:      tag,
		Metadata: md,
		Payload:  data,
	}

	s.client.Logger().Debug("broadcast", "tag", tag, "data", data)
	return s.client.WriteFrame(f)
}

func buildDadaFrame(clientID string, broadcast bool, tid string, tag uint32, data []byte) (*frame.DataFrame, error) {
	md, err := core.NewDefaultMetadata(clientID, broadcast, tid).Encode()
	if err != nil {
		return nil, err
	}
	f := &frame.DataFrame{
		Tag:      tag,
		Metadata: md,
		Payload:  data,
	}
	return f, nil
}
