package trace

import (
	"context"
	"errors"
	"log"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
)

// tracerProvider returns an OpenTelemetry TracerProvider configured to use
// the Jaeger exporter that will send spans to the provided url. The returned
// TracerProvider will also use a Resource configured with all the information
// about the application.
func tracerProvider(service string, exp tracesdk.SpanExporter) *tracesdk.TracerProvider {
	tp := tracesdk.NewTracerProvider(
		// Always be sure to batch in production.
		tracesdk.WithBatcher(exp),
		tracesdk.WithSampler(tracesdk.AlwaysSample()),
		// tracesdk.WithSyncer(exp),
		// Record information about this application in an Resource.
		tracesdk.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(service),
			// attribute.String("environment", environment),
			// attribute.Int64("ID", id),
		)),
	)
	return tp
}

// NewTracerProviderWithJaeger creates a new tracer provider with Jaeger.
func NewTracerProviderWithJaeger(service string) (*tracesdk.TracerProvider, func(ctx context.Context), error) {
	// tracer provider
	endpoint := os.Getenv("YOMO_TRACE_JAEGER_ENDPOINT")
	if endpoint == "" {
		return nil, func(context.Context) {}, errors.New("tracing disabled")
	}
	// Create the Jaeger exporter
	exp, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(endpoint)))
	if err != nil {
		return nil, func(context.Context) {}, err
	}
	// tracer provider
	tp := tracerProvider(service, exp)
	// shutdown
	shutdown := func(ctx context.Context) {
		// Do not make the application hang when it is shutdown.
		ctx, cancel := context.WithTimeout(ctx, time.Second*5)
		defer cancel()
		if err := tp.Shutdown(ctx); err != nil {
			log.Printf("[trace] shutdonw err: %v\n", err)
		}
	}
	// Register our TracerProvider as the global so any imported
	// instrumentation in the future will default to using it.
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	// otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	return tp, shutdown, nil
}

// NewSpan creates a new span of OpenTelemetry.
func NewSpan(tp trace.TracerProvider, tracerName string, spanName string, traceID string, spanID string) (trace.Span, error) {
	return NewSpanWithAttrs(tp, tracerName, spanName, traceID, spanID, false)
}

// NewTraceSpan creates a new span of OpenTelemetry from remote parent tracing.
func NewRemoteSpan(tp trace.TracerProvider, tracerName string, spanName string, traceID string, spanID string) (trace.Span, error) {
	return NewSpanWithAttrs(tp, tracerName, spanName, traceID, spanID, true)
}

// NewSpanWithAttrs creates a new span of OpenTelemetry with attributes.
func NewSpanWithAttrs(tp trace.TracerProvider, tracerName string, spanName string, traceID string, spandID string, remote bool, attrs ...map[string]string) (trace.Span, error) {
	if tp == nil {
		return nil, errors.New("tracer provider is nil")
	}
	ctx := context.Background()
	// root span
	if traceID == "" && spandID == "" {
		tr := tp.Tracer(tracerName)
		_, span := tr.Start(ctx, spanName)
		if len(attrs) > 0 {
			for k, v := range attrs[0] {
				span.SetAttributes(attribute.Key(k).String(v))
			}
		}
		return span, nil
	}
	// child span
	tid, err := trace.TraceIDFromHex(traceID)
	if err != nil {
		return nil, err
	}
	sid, err := trace.SpanIDFromHex(spandID)
	if err != nil {
		return nil, err
	}
	scc := trace.SpanContextConfig{
		TraceID: tid,
		SpanID:  sid,
		Remote:  remote,
	}
	ctx = trace.ContextWithRemoteSpanContext(ctx, trace.NewSpanContext(scc))
	tr := tp.Tracer(tracerName)
	_, span := tr.Start(ctx, spanName)
	return span, nil
}
