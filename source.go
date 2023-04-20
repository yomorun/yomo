package yomo

import (
	"context"

	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/frame"
)

// Source is responsible for sending data to yomo.
type Source interface {
	// Close will close the connection to YoMo-Zipper.
	Close() error
	// Connect to YoMo-Zipper.
	Connect() error
	// SetDataTag will set the tag of data when invoking Write().
	SetDataTag(tag frame.Tag)
	// Write the data to directed downstream.
	Write(data []byte) (n int, err error)
	// WriteWithTag will write data with specified tag, default transactionID is epoch time.
	WriteWithTag(tag frame.Tag, data []byte) error
	// SetErrorHandler set the error handler function when server error occurs
	SetErrorHandler(fn func(err error))
	// [Experimental] SetReceiveHandler set the observe handler function
	SetReceiveHandler(fn func(tag frame.Tag, data []byte))
	// Write the data to all downstream
	Broadcast(data []byte) error
	// NewStream will create a new data stream [experimental feature]
	NewStream(metadata []byte) (core.DataStream, error)
}

// YoMo-Source
type yomoSource struct {
	name       string
	zipperAddr string
	client     *core.Client
	tag        frame.Tag
	fn         func(frame.Tag, []byte)
}

var _ Source = &yomoSource{}

// NewSource create a yomo-source
func NewSource(name, zipperAddr string, opts ...SourceOption) Source {
	client := core.NewClient(name, core.ClientTypeSource, opts...)

	return &yomoSource{
		name:       name,
		zipperAddr: zipperAddr,
		client:     client,
	}
}

// Write the data to downstream.
func (s *yomoSource) Write(data []byte) (int, error) {
	err := s.WriteWithTag(s.tag, data)
	if err != nil {
		return 0, err
	}
	return len(data), nil
}

// SetDataTag will set the tag of data when invoking Write().
func (s *yomoSource) SetDataTag(tag frame.Tag) {
	s.tag = tag
}

// Close will close the connection to YoMo-Zipper.
func (s *yomoSource) Close() error {
	if err := s.client.Close(); err != nil {
		s.client.Logger().Error("failed to close the source", err)
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
			s.fn(frm.GetDataTag(), frm.GetCarriage())
		}
	})

	err := s.client.Connect(context.Background(), s.zipperAddr)
	return err
}

// WriteWithTag will write data with specified tag, default transactionID is epoch time.
func (s *yomoSource) WriteWithTag(tag frame.Tag, data []byte) error {
	f := frame.NewDataFrame()
	f.SetCarriage(tag, data)
	f.SetSourceID(s.client.ClientID())
	s.client.Logger().Debug("source write with tag", "data_frame", f.String())
	return s.client.WriteFrame(f)
}

// SetErrorHandler set the error handler function when server error occurs
func (s *yomoSource) SetErrorHandler(fn func(err error)) {
	s.client.SetErrorHandler(fn)
}

// [Experimental] SetReceiveHandler set the observe handler function
func (s *yomoSource) SetReceiveHandler(fn func(frame.Tag, []byte)) {
	s.fn = fn
	s.client.Logger().Info("receive hander set for the source")
}

// Broadcast write the data to all downstreams.
func (s *yomoSource) Broadcast(data []byte) error {
	f := frame.NewDataFrame()
	f.SetCarriage(s.tag, data)
	f.SetSourceID(s.client.ClientID())
	f.SetBroadcast(true)
	s.client.Logger().Debug("broadcast", "data_frame", f.String())
	return s.client.WriteFrame(f)
}

// NewStream will create a new data stream [experimental feature]
func (s *yomoSource) NewStream(metadata []byte) (core.DataStream, error) {
	return s.client.NewStream(metadata)
}
