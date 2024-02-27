package trace

import (
	"context"
	"log"
	"os"
	"sync/atomic"

	"github.com/yomorun/yomo/core/metadata"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

var serviceName atomic.Value

func init() {
	defaultServiceName := "yomo"
	serviceName.Store(defaultServiceName)

	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		otel.SetTracerProvider(noop.NewTracerProvider())
	} else {
		setTracerProvider(defaultServiceName)
	}
}

// SetServiceName set service name for tracing.
func SetServiceName(name string) {
	setTracerProvider(serviceName.Load().(string))
}

// NewTracerProvider returns an OpenTelemetry TracerProvider configured to use
// the Jaeger exporter that will send spans to the provided url. The returned
// TracerProvider will also use a Resource configured with all the information
// about the application.
func NewTracerProvider(service string) *tracesdk.TracerProvider {
	client := otlptracehttp.NewClient()
	exp, err := otlptrace.New(context.Background(), client)
	if err != nil {
		panic("failed to create trace exporter: " + err.Error())
	}
	tp := tracesdk.NewTracerProvider(
		tracesdk.WithBatcher(exp),
		tracesdk.WithSampler(tracesdk.AlwaysSample()),
		tracesdk.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(service),
		)),
	)
	return tp
}

// Tracer is otel span tracer.
type Tracer struct {
	tracer         trace.Tracer
	tracerName     string
	tracerProvider trace.TracerProvider
}

// NewTracer create tracer instance.
func NewTracer(name string, tp trace.TracerProvider) *Tracer {
	if tp == nil {
		tp = otel.GetTracerProvider()
	}
	return &Tracer{
		tracer:         tp.Tracer(name),
		tracerName:     name,
		tracerProvider: tp,
	}
}

// Start start tracing span.
func (t *Tracer) Start(md metadata.M, operation string) trace.Span {
	_, span := t.tracer.Start(NewContextWithMetadata(md),
		operation,
	)
	return span
}

// End finish tracing span.
func (t *Tracer) End(md metadata.M, span trace.Span, kv ...attribute.KeyValue) {
	// use metadata to propagate the trace info.
	if span.SpanContext().TraceID().IsValid() {
		md.Set(metadata.TraceIDKey, span.SpanContext().TraceID().String())
	}

	if span.SpanContext().SpanID().IsValid() {
		md.Set(metadata.SpanIDKey, span.SpanContext().SpanID().String())
	}

	for _, v := range kv {
		span.SetAttributes(v)
	}
	span.End()
}

// NewContextWithMetadata create new context with metadata for tracer starting.
// In yomo, we use metadata from dataFrame as the trace Propagator. And yomo only
// carries traceID and spanID in metadata.
func NewContextWithMetadata(md metadata.M) context.Context {
	traceID, ok := md.Get(metadata.TraceIDKey)
	if !ok {
		return context.Background()
	}
	spanID, ok := md.Get(metadata.SpanIDKey)
	if !ok {
		return context.Background()
	}

	tid, err := trace.TraceIDFromHex(traceID)
	if err != nil {
		return context.Background()
	}
	sid, err := trace.SpanIDFromHex(spanID)
	if err != nil {
		return context.Background()
	}

	scc := trace.SpanContextConfig{
		TraceID: tid,
		SpanID:  sid,
	}
	spanContext := trace.NewSpanContext(scc)

	return trace.ContextWithSpanContext(context.Background(), spanContext)
}

func setTracerProvider(service string) {
	client := otlptracehttp.NewClient()
	exp, err := otlptrace.New(context.Background(), client)
	if err != nil {
		log.Println("init otlp client error, use noop provider, err:", err)
		otel.SetTracerProvider(noop.NewTracerProvider())
		return
	}

	tp := tracesdk.NewTracerProvider(
		tracesdk.WithBatcher(exp),
		tracesdk.WithSampler(tracesdk.AlwaysSample()),
		tracesdk.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(service),
		)),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
}
