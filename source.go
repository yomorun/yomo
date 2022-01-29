package yomo

import (
	"context"

	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/pkg/logger"
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
	// WriteDataFrame will write data frame to zipper.
	WriteDataFrame(f *frame.DataFrame) error
}

// YoMo-Source
type yomoSource struct {
	name           string
	zipperEndpoint string
	client         *core.Client
	tag            uint8
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
	logger.Debugf("%s\tWrite: data=%# x", sourceLogPrefix, data)
	return len(data), s.WriteWithTag(s.tag, data)
}

// SetDataTag will set the tag of data when invoking Write().
func (s *yomoSource) SetDataTag(tag uint8) {
	s.tag = tag
}

// Close will close the connection to YoMo-Zipper.
func (s *yomoSource) Close() error {
	if err := s.client.Close(); err != nil {
		logger.Errorf("%sClose(): %v", sourceLogPrefix, err)
		return err
	}
	logger.Debugf("%s is closed", sourceLogPrefix)
	return nil
}

// Connect to YoMo-Zipper.
func (s *yomoSource) Connect() error {
	err := s.client.Connect(context.Background(), s.zipperEndpoint, nil)
	if err != nil {
		logger.Errorf("%sConnect() error: %s", sourceLogPrefix, err)
	}
	return err
}

// WriteWithTag will write data with specified tag, default transactionID is epoch time.
func (s *yomoSource) WriteWithTag(tag uint8, data []byte) error {
	f := frame.NewDataFrame()
	f.SetCarriage(byte(tag), data)
	return s.WriteDataFrame(f)
}

// WriteDataFrame will write data frame to zipper.
func (s *yomoSource) WriteDataFrame(f *frame.DataFrame) error {
	if len(f.GetCarriage()) > 1024 {
		logger.Debugf("%sWriteDataFrame: len(data)=%d", sourceLogPrefix, len(f.GetCarriage()))
	} else {
		logger.Debugf("%sWriteDataFrame: data=%# x", sourceLogPrefix, f.GetCarriage())
	}
	return s.client.WriteFrame(f)
}
