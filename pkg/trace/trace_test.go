package trace

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core/metadata"
)

func TestTraceProvider(t *testing.T) {
	os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:43118")
	SetTracerProvider("yomo-test")

	tracer := NewTracer("Source")

	md := metadata.New()

	span := tracer.Start(md, "source")
	time.Sleep(time.Millisecond * 100)
	tracer.End(md, span)
	tid1 := assertMd(t, md)

	tracer = NewTracer("Zipper")

	span = tracer.Start(md, "zipper-endpoint")
	time.Sleep(time.Millisecond * 150)
	tracer.End(md, span)
	tid2 := assertMd(t, md)

	tracer = NewTracer("StreamFunction")

	span = tracer.Start(md, "sink")
	time.Sleep(time.Millisecond * 200)
	tracer.End(md, span)
	tid3 := assertMd(t, md)

	assert.True(t, tid1 == tid2 && tid2 == tid3 && tid3 == tid1)
	ShutdownTracerProvider()
}

func assertMd(t *testing.T, md metadata.M) string {
	traceID, ok1 := md.Get(metadata.TraceIDKey)
	assert.True(t, ok1)
	assert.Equal(t, 32, len(traceID))

	spanID, ok2 := md.Get(metadata.SpanIDKey)
	assert.True(t, ok2)
	assert.Equal(t, 16, len(spanID))

	return traceID
}
