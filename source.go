package yomo

import (
	"context"

	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/frame"
)

const (
	sourceLogPrefix = "\033[32m[yomo:source]\033[0m "
)

// Source is responsible for sending data to yomo.
type Source interface {
	// Close will close the connection to YoMo-Zipper.
	Close() error
	// Connect to YoMo-Zipper.
	Connect() error
	// SetDataTag will set the tag of data when invoking Write().
	SetDataTag(tag uint8)
	// Write the data to downstream.
	Write(p []byte) (n int, err error)
	// WriteWithTag will write data with specified tag, default transactionID is epoch time.
	WriteWithTag(tag uint8, data []byte) error
	// SetErrorHandler set the error handler function when server error occurs
	SetErrorHandler(fn func(err error))
	// [Experimental] SetReceiveHandler set the observe handler function
	SetReceiveHandler(fn func(tag byte, data []byte))
}

// YoMo-Source
type yomoSource struct {
	name           string
	zipperEndpoint string
	client         *core.Client
	tag            uint8
	fn             func(byte, []byte)
}

var _ Source = &yomoSource{}

// NewSource create a yomo-source
func NewSource(name string, opts ...Option) Source {
	options := NewOptions(opts...)
	client := core.NewClient(name, core.ClientTypeSource, options.ClientOptions...)

	return &yomoSource{
		name:           name,
		zipperEndpoint: options.ZipperAddr,
		client:         client,
	}
}

// Write the data to downstream.
func (s *yomoSource) Write(data []byte) (int, error) {
	return len(data), s.WriteWithTag(s.tag, data)
}

// SetDataTag will set the tag of data when invoking Write().
func (s *yomoSource) SetDataTag(tag uint8) {
	s.tag = tag
}

// Close will close the connection to YoMo-Zipper.
func (s *yomoSource) Close() error {
	if err := s.client.Close(); err != nil {
		s.client.Logger().Errorf("%sClose(): %v", sourceLogPrefix, err)
		return err
	}
	s.client.Logger().Debugf("%s is closed", sourceLogPrefix)
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

	err := s.client.Connect(context.Background(), s.zipperEndpoint)
	if err != nil {
		s.client.Logger().Errorf("%sConnect() error: %s", sourceLogPrefix, err)
	}
	return err
}

// WriteWithTag will write data with specified tag, default transactionID is epoch time.
func (s *yomoSource) WriteWithTag(tag uint8, data []byte) error {
	f := frame.NewDataFrame()
	f.SetCarriage(byte(tag), data)
	f.SetSourceID(s.client.ClientID())
	s.client.Logger().Debugf("%sWriteWithTag: tid=%s, source_id=%s, data[%d]=%# x",
		sourceLogPrefix, f.TransactionID(), f.SourceID(), len(data), frame.Shortly(data))
	return s.client.WriteFrame(f)
}

// SetErrorHandler set the error handler function when server error occurs
func (s *yomoSource) SetErrorHandler(fn func(err error)) {
	s.client.SetErrorHandler(fn)
}

// [Experimental] SetReceiveHandler set the observe handler function
func (s *yomoSource) SetReceiveHandler(fn func(byte, []byte)) {
	s.fn = fn
	s.client.Logger().Debugf("%sSetReceiveHandler(%v)", sourceLogPrefix, s.fn)
}
