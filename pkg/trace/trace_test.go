package trace

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/yomorun/yomo/core/metadata"
)

func TestTrace(t *testing.T) {
	os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318")
	tp := NewTracerProvider("yomo-test")

	tracer := NewTracer("Source", tp)

	md := metadata.New()

	span := tracer.Start(md, "write-to")
	time.Sleep(time.Second)
	tracer.End(md, span)
	t.Log(md)

	tracer = NewTracer("Zipper", tp)

	span = tracer.Start(md, "zipper-endpoint")
	time.Sleep(time.Second)
	tracer.End(md, span)
	t.Log(md)

	tracer = NewTracer("StreamFunction", tp)

	span = tracer.Start(md, "sink")
	time.Sleep(time.Second)
	tracer.End(md, span)
	t.Log(md)

	tp.ForceFlush(context.TODO())
}
