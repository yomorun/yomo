package yomo

import (
	"context"

	"github.com/yomorun/yomo/internal/core"
	"github.com/yomorun/yomo/internal/frame"
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
	options := newOptions(opts...)
	client := core.NewClient(name, core.ClientTypeSource)

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
		logger.Errorf("%sClose(): %v", sourceLogPrefix, err)
		return err
	}
	logger.Debugf("%s is closed", sourceLogPrefix)
	return nil
}

// Connect to YoMo-Zipper.
func (s *yomoSource) Connect() error {
	err := s.client.Connect(context.Background(), s.zipperEndpoint)
	if err != nil {
		logger.Errorf("%sConnect() error: %s", sourceLogPrefix, err)
	}
	return err
}

// WriteWithTag will write data with specified tag, default transactionID is epoch time.
func (s *yomoSource) WriteWithTag(tag uint8, data []byte) error {
	if len(data) > 1024 {
		logger.Debugf("%sWriteDataWithTransactionID: len(data)=%d", sourceLogPrefix, len(data))
	} else {
		logger.Debugf("%sWriteDataWithTransactionID: data=%# x", sourceLogPrefix, data)
	}
	frame := frame.NewDataFrame()
	frame.SetCarriage(byte(tag), data)
	return s.client.WriteFrame(frame)
}
