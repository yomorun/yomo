package yomo

import (
	"context"

	"github.com/yomorun/yomo/internal/core"
	"github.com/yomorun/yomo/logger"
)

const (
	ZipperLogPrefix = "\033[33m[yomo:zipper]\033[0m "
)

type Zipper interface {
	ConfigWorkflow(conf string) error
	AddWorkflow(wf ...core.Workflow) error
	ConfigDownstream(opts ...interface{}) error
	ListenAndServe() error
	Connect() error
	AddDownstreamZipper(downstream Zipper) error
	RemoveDownstreamZipper(downstream Zipper) error
	Endpoint() string
	Stats() int
	Close() error
}

// Zipper 有两种存在形式：UpstreamZipper（QUIC Client端） 和 DownstreamZipper（QUIC Server端）。
// Upstream 可以同时连接到多个 Downstream，数据由 Upstream 被分发给下游的多个 Downstream。
type zipper struct {
	token             string
	endpoint          string
	hasDownstreams    bool
	server            *core.Server
	client            *core.Client
	downstreamZippers []Zipper
	// logger            utils.Logger
}

var _ Zipper = &zipper{}

// 创建下游级联的 Zipper
func NewZipper(opts ...Option) *zipper {
	options := newOptions(opts...)
	client := core.NewClient(options.AppName, core.ConnTypeUpstreamZipper)

	return &zipper{
		token:    options.AppName,
		endpoint: options.ZipperEndpoint,
		client:   client,
	}
}

// 创建 Zipper 服务端，
// 会接入 Source, Upstream Zipper, Sfn
func NewZipperServer(opts ...Option) Zipper {
	options := newOptions(opts...)
	srv := core.NewServer()

	return &zipper{
		token:          options.AppName,
		endpoint:       options.ZipperEndpoint,
		hasDownstreams: false,
		server:         srv,
		// logger:         utils.DefaultLogger.WithPrefix("\033[33m[yomo:zipper]\033[0m"),
	}
}

/*************** Server ONLY ***************/

// 读取 workflow.yaml 配置文件，
// CLI： 读取 workflow.yaml，
// Cloud：从 Database 或 API 读取
func (s *zipper) ConfigWorkflow(conf string) error {
	config, err := ParseConfig(conf)
	if err != nil {
		return err
	}
	for i, app := range config.Functions {
		if err := s.server.AddWorkflow(core.Workflow{Seq: i, Token: app.Name}); err != nil {
			return err
		}
	}
	return nil
}

func (s *zipper) AddWorkflow(wfs ...core.Workflow) error {
	return s.server.AddWorkflow(wfs...)
}

// 读取下游downstream的配置
// CLI：通过 HTTP 请求读取（目前还未添加 auth 相关的功能，安全上会是个问题）
// Cloud：从 Database 或 API 读取
func (s *zipper) ConfigDownstream(opts ...interface{}) error {
	return nil
}

// 启动 Zipper Server，
// 将 Source 的内容：
//    1、按顺序传递个 sfn，
//    2、并行传输给 Downstream Zipper
func (s *zipper) ListenAndServe() error {
	logger.Debugf("%sCreating Zipper Server ...", ZipperLogPrefix)
	return s.server.ListenAndServe(context.Background(), s.endpoint)
}

/*************** Client ONLY ****************************
/* 在 Zipper 级联场景中，Zipper 本身也可以作为 QUIC Client，
/* 此时，我们称该种 Zipper 为 Upstream Zipper。
*********************************************************/

// Client：建立到 downstream zipper 的连接
func (s *zipper) Connect() error {
	err := s.client.Connect(context.Background(), s.endpoint)
	if err != nil {
		logger.Errorf("%sConnect() error: %s", ZipperLogPrefix, err)
		return err
	}

	return nil
}

// Client：添加 upstream zipper
func (s *zipper) AddDownstreamZipper(downstream Zipper) error {
	logger.Debugf("%sAddDownstreamZipper: %v", ZipperLogPrefix, downstream)
	s.downstreamZippers = append(s.downstreamZippers, downstream)
	s.hasDownstreams = true
	logger.Debugf("%scurrent downstreams: %v", ZipperLogPrefix, s.downstreamZippers)
	return nil
}

// Client：去除 downstream Zipper
func (s *zipper) RemoveDownstreamZipper(downstream Zipper) error {
	index := -1
	for i, v := range s.downstreamZippers {
		if v.Endpoint() == downstream.Endpoint() {
			index = i
			break
		}
	}

	// remove from slice
	s.downstreamZippers = append(s.downstreamZippers[:index], s.downstreamZippers[index+1:]...)
	return nil
}

// 获取 zipper 监听的 endpoint
func (s *zipper) Endpoint() string {
	return s.endpoint
}

/*************** Client/Server 都用 ***************/

// Client：关闭本地连接；
// Server：关闭服务器；
func (s *zipper) Close() error {
	if s.server != nil {
		if err := s.server.Close(); err != nil {
			logger.Errorf("%s Close(): %v", ZipperLogPrefix, err)
			return err
		}
	}
	if s.client != nil {
		if err := s.client.Close(); err != nil {
			logger.Errorf("%s Close(): %v", ZipperLogPrefix, err)
			return err
		}
	}
	return nil
}

func (s *zipper) Stats() int {
	logger.Debugf("%sall sfn connected: %d", ZipperLogPrefix, len(s.server.StatsFunctions()))
	for k, v := range s.server.StatsFunctions() {
		logger.Debugf("%s%s-> k=%v, v.StreamID=%d", ZipperLogPrefix, k, (*v).StreamID())
	}

	logger.Debugf("%stotal DataFrames received: %d", ZipperLogPrefix, s.server.StatsCounter())
	return len(s.server.StatsFunctions())
}
