package yomo

import (
	"context"

	"github.com/yomorun/yomo/internal/core"
	"github.com/yomorun/yomo/internal/frame"
	"github.com/yomorun/yomo/logger"
)

const (
	StreamFunctionLogPrefix = "\033[31m[yomo:sfn]\033[0m "
)

type StreamFunction interface {
	SetObserveDataID(id ...uint8)
	SetHandler(fn func([]byte) (byte, []byte)) error
	Connect() error
	Close() error
}

// NewStreamFunction create a stream function
func NewStreamFunction(opts ...Option) *streamFunction {
	options := newOptions(opts...)
	client := core.NewClient(options.AppName, core.ConnTypeStreamFunction)
	sfn := streamFunction{
		name:           options.AppName,
		zipperEndpoint: options.ZipperEndpoint,
		client:         client,
		observed:       make([]uint8, 0),
		// logger:         utils.DefaultLogger.WithPrefix("\033[31m[yomo:sfn]\033[0m"),
	}

	return &sfn
}

var _ StreamFunction = &streamFunction{}

// Steaming StreamFunction 定义
type streamFunction struct {
	name           string
	zipperEndpoint string
	client         *core.Client
	// logger         utils.Logger
	observed []uint8                     // StreamFunction 监听的数据 ID
	fn       func([]byte) (byte, []byte) // StreamFunction 的方法
}

// 设置监听的 DataID 列表，当列表中对应 ID 的数据到达时，将其放入 RxStream 中，
// 只有一个 RxStream 对象，用户可以通过 RxStream.Map() 方法再繁殖出新的逻辑流，
// 进而再进行操作
func (s *streamFunction) SetObserveDataID(id ...uint8) {
	s.observed = append(s.observed, id...)
	logger.Debugf("%sSetObserveDataID(%v)", StreamFunctionLogPrefix, s.observed)
}

// 注入 Handler() 回调
func (s *streamFunction) SetHandler(fn func([]byte) (byte, []byte)) error {
	s.fn = fn
	return nil
}

// 开始连接到 Zipper，接收到的数据将被 SetHandler() 方法注入的 func_ 处理
func (s *streamFunction) Connect() error {
	// 注册给底层的 quic-client，当收到 DataFrame 时，转发过来
	s.client.SetDataFrameObserver(func(tag byte, carraige []byte) {
		for _, t := range s.observed {
			if t == tag {
				logger.Debugf("%sreceive DataFrame, tag=%# x, carraige=%# x", StreamFunctionLogPrefix, tag, carraige)
				s.onDataFrame(carraige)
				return
			}
		}
	})

	err := s.client.Connect(context.Background(), s.zipperEndpoint)
	if err != nil {
		// 创建连接失败
		logger.Errorf("%sConnect() error: %s", StreamFunctionLogPrefix, err)
		return err
	}
	return nil
}

// 关闭连接
func (s *streamFunction) Close() error {
	if s.client != nil {
		if err := s.client.Close(); err != nil {
			logger.Errorf("%sClose(): %v", err)
			return err
		}
	}
	return nil
}

func (s *streamFunction) onDataFrame(data []byte) {
	if s.fn == nil {
		logger.Warnf("%sStreamFunction is nil", StreamFunctionLogPrefix)
		return
	}
	logger.Debugf("%sexecute-start fn: data=%# x", StreamFunctionLogPrefix, data)
	tag, resp := s.fn(data)
	logger.Debugf("%sexecute-done fn: tag=%# x, resp=%# x", StreamFunctionLogPrefix, tag, resp)
	// resp 是用户返回的数据，如果不为空，要发送回给 zipper
	if len(resp) != 0 {
		logger.Debugf("%s\tstart WriteFrame(): tag=%# x, data=%v", StreamFunctionLogPrefix, tag, resp)
		transactionID := "tid-sfn-"
		frame := frame.NewDataFrame(transactionID, s.name)
		frame.SetCarriage(tag, resp)
		s.client.WriteFrame(frame)
	}
}
