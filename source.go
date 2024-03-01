package yomo

import (
	"context"

	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/pkg/id"
	"github.com/yomorun/yomo/pkg/trace"
	"go.opentelemetry.io/otel/attribute"
)

// Source is responsible for sending data to yomo.
type Source interface {
	// Close will close the connection to YoMo-Zipper.
	Close() error
	// Connect to YoMo-Zipper.
	Connect() error
	// Write the data to directed downstream.
	Write(tag uint32, data []byte) error
	// WriteWithTarget writes data to sfn instance with specified target.
	WriteWithTarget(tag uint32, data []byte, target string) error
	// SetErrorHandler set the error handler function when server error occurs
	SetErrorHandler(fn func(err error))
}

// YoMo-Source
type yomoSource struct {
	name       string
	zipperAddr string
	client     *core.Client
}

var _ Source = &yomoSource{}

// NewSource create a yomo-source
func NewSource(name, zipperAddr string, opts ...SourceOption) Source {
	clientOpts := make([]core.ClientOption, len(opts))
	for k, v := range opts {
		clientOpts[k] = core.ClientOption(v)
	}

	client := core.NewClient(name, zipperAddr, core.ClientTypeSource, clientOpts...)

	client.Logger = client.Logger.With(
		"component", core.ClientTypeSource.String(),
		"source_id", client.ClientID(),
		"source_name", client.Name(),
		"zipper_addr", zipperAddr,
	)

	return &yomoSource{
		name:       name,
		zipperAddr: zipperAddr,
		client:     client,
	}
}

// Close will close the connection to YoMo-Zipper.
func (s *yomoSource) Close() error {
	if err := s.client.Close(); err != nil {
		s.client.Logger.Error("failed to close the source", "err", err)
		return err
	}
	trace.ShutdownTracerProvider()
	s.client.Logger.Debug("the source is closed")
	return nil
}

// Connect to YoMo-Zipper.
func (s *yomoSource) Connect() error {
	return s.client.Connect(context.Background())
}

// Write writes data with specified tag.
func (s *yomoSource) Write(tag uint32, data []byte) error {
	md := core.NewMetadata(s.client.ClientID(), id.New())
	// add trace
	tracer := trace.NewTracer("Source")
	span := tracer.Start(md, s.name)
	defer tracer.End(
		md,
		span,
		attribute.Int("send_data_tag", int(tag)),
		attribute.Int("send_data_len", len(data)),
	)

	mdBytes, err := md.Encode()
	// metadata
	if err != nil {
		return err
	}
	f := &frame.DataFrame{
		Tag:      tag,
		Metadata: mdBytes,
		Payload:  data,
	}
	s.client.Logger.Debug("source write", "tag", tag, "data", data)
	return s.client.WriteFrame(f)
}

// WritePayload writes `yomo.Payload` with specified tag.
func (s *yomoSource) WriteWithTarget(tag uint32, data []byte, target string) error {
	if data == nil {
		return nil
	}
	md := core.NewMetadata(s.client.ClientID(), id.New())
	// add trace
	tracer := trace.NewTracer("Source")
	span := tracer.Start(md, s.name)
	defer tracer.End(
		md,
		span,
		attribute.Int("send_data_tag", int(tag)),
		attribute.String("send_data_target", target),
		attribute.Int("send_data_len", len(data)),
	)

	if target != "" {
		core.SetMetadataTarget(md, target)
	}

	mdBytes, err := md.Encode()
	if err != nil {
		return err
	}
	f := &frame.DataFrame{
		Tag:      tag,
		Metadata: mdBytes,
		Payload:  data,
	}
	s.client.Logger.Debug("source write with target", "tag", tag, "data", data, "target", target)
	return s.client.WriteFrame(f)
}

// SetErrorHandler set the error handler function when server error occurs
func (s *yomoSource) SetErrorHandler(fn func(err error)) {
	s.client.SetErrorHandler(fn)
}
