// Package trace provides otel span tracer for YoMo's stream function.
package trace

import (
	"context"
	"log"
	"os"

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

var (
	// ServiceName is the default service name for otel.
	ServiceName = "yomo"
)

// SetTracerProvider set otel tracer provider.
// if enveronment BASELIME_API_KEY is set, the tracer provider will be baselime tracer provider.
// if enveronment OTEL_EXPORTER_OTLP_ENDPOINT is set, the tracer provider will be otlptracehttp tracer provider.
// This function set the global tracer provider by calling otel.SetTracerProvider(),
// User also can set other tracer provider by calling otel.SetTracerProvider()
func SetTracerProvider() {
	client := NewClientFromEnv()
	if client == nil {
		otel.SetTracerProvider(noop.NewTracerProvider())
		return
	}
	tp := NewTracerProviderFromClient(context.Background(), ServiceName, client)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
}

// NewClientFromEnv create otlptrace.Client from environment.
func NewClientFromEnv() otlptrace.Client {
	if baselimeApiKey, ok := os.LookupEnv("BASELIME_API_KEY"); ok {
		return otlptracehttp.NewClient(
			otlptracehttp.WithEndpointURL("https://otel.baselime.io"),
			otlptracehttp.WithHeaders(map[string]string{"x-api-key": baselimeApiKey}),
		)
	}
	if endpoint, ok := os.LookupEnv("OTEL_EXPORTER_OTLP_ENDPOINT"); ok {
		return otlptracehttp.NewClient(otlptracehttp.WithEndpointURL(endpoint))
	}
	return nil
}

// NewTracerProviderFromClient create tracer provider from otlptrace.Client.
func NewTracerProviderFromClient(ctx context.Context, serviceName string, client otlptrace.Client) *tracesdk.TracerProvider {
	exporter, err := otlptrace.New(ctx, client)
	if err != nil {
		log.Fatalln("failed to create trace exporter: " + err.Error())
	}
	return tracesdk.NewTracerProvider(
		tracesdk.WithBatcher(exporter),
		tracesdk.WithSampler(tracesdk.AlwaysSample()),
		tracesdk.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(serviceName),
		)),
	)
}

// ShutdownTracerProvider shutdown the global TracerProvider.
func ShutdownTracerProvider() {
	tp := otel.GetTracerProvider()
	switch i := tp.(type) {
	case *tracesdk.TracerProvider:
		ctx, cancel := context.WithTimeout(context.Background(), 5)
		defer cancel()
		i.Shutdown(ctx)
	case *noop.TracerProvider:
		return
	}
}

// Tracer is otel span tracer.
type Tracer struct {
	tracer         trace.Tracer
	tracerName     string
	tracerProvider trace.TracerProvider
}

// NewTracer create tracer instance.
func NewTracer(name string, enable ...bool) *Tracer {
	tp := otel.GetTracerProvider()
	if len(enable) > 0 && !enable[0] {
		tp = noop.NewTracerProvider()
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
	propagateTrace(md, span)
	return span
}

// yomo uses metadata to propagate the trace info.
func propagateTrace(md metadata.M, span trace.Span) {
	if span.SpanContext().TraceID().IsValid() {
		md.Set(metadata.TraceIDKey, span.SpanContext().TraceID().String())
	}

	if span.SpanContext().SpanID().IsValid() {
		md.Set(metadata.SpanIDKey, span.SpanContext().SpanID().String())
	}
}

// End finish tracing span.
func (t *Tracer) End(md metadata.M, span trace.Span, kv ...attribute.KeyValue) {
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
