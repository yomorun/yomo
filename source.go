package yomo

import (
	"context"
	"strconv"
	"time"

	"github.com/yomorun/yomo/internal/core"
	"github.com/yomorun/yomo/internal/frame"
	"github.com/yomorun/yomo/logger"
)

const (
	SourceLogPrefix = "\033[32m[yomo:source]\033[0m "
)

type Source interface {
	Write(p []byte) (n int, err error)
	SetDataTag(tag uint8)
	Close() error
	Connect() error
	WriteWithTag(tag uint8, data []byte) error
	WriteWithTransaction(transactionID string, tag uint8, data []byte) error
}

// YoMo-Source
type yomoSource struct {
	name           string
	zipperEndpoint string
	client         *core.Client
	// logger         utils.Logger
	tag uint8
}

var _ Source = &yomoSource{}

// NewSource create a yomo-source
func NewSource(opts ...Option) Source {
	options := newOptions(opts...)
	client := core.NewClient(options.AppName, core.ConnTypeSource)

	return &yomoSource{
		name:           options.AppName,
		zipperEndpoint: options.ZipperEndpoint,
		client:         client,
	}
}

// Write the data to downstream.
func (s *yomoSource) Write(data []byte) (int, error) {
	return len(data), s.WriteWithTag(s.tag, data)
}

func (s *yomoSource) SetDataTag(tag uint8) {
	s.tag = tag
}

func (s *yomoSource) Close() error {
	if err := s.client.Close(); err != nil {
		logger.Errorf("%sClose(): %v", SourceLogPrefix, err)
		return err
	}
	logger.Debugf("%s is closed", SourceLogPrefix)
	return nil
}

// Connect to YoMo-Zipper.
func (s *yomoSource) Connect() error {
	err := s.client.Connect(context.Background(), s.zipperEndpoint)
	if err != nil {
		logger.Errorf("%sConnect() error: %s", SourceLogPrefix, err)
		return err
	}
	return nil
}

// 向 Zipper 发送用户数据，指定该次发送数据使用的 tag
func (s *yomoSource) WriteWithTag(tag uint8, data []byte) error {
	transactionID := strconv.FormatInt(time.Now().UnixNano(), 10)
	return s.WriteWithTransaction(transactionID, tag, data)
}

// 向 Zipper 发送用户数据，指定该次发送数据使用的 tag 以及 transactionID
func (s *yomoSource) WriteWithTransaction(transactionID string, tag uint8, data []byte) error {
	if len(data) > 1024 {
		logger.Debugf("%sWriteDataWithTransactionID: len(data)=%d", SourceLogPrefix, len(data))
	} else {
		logger.Debugf("%sWriteDataWithTransactionID: data=%# x", SourceLogPrefix, data)
	}
	frame := frame.NewDataFrame(transactionID, s.name)
	frame.SetCarriage(byte(tag), data)

	return s.client.WriteFrame(frame)
}
