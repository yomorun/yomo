package tracing

// import (
// 	"context"
// 	"errors"
// 	"log"
// 	"os"
// 	"strconv"
// 	"time"

// 	"github.com/tidwall/gjson"
// 	"go.opentelemetry.io/otel"
// 	"go.opentelemetry.io/otel/exporters/jaeger"
// 	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
// 	"go.opentelemetry.io/otel/propagation"
// 	"go.opentelemetry.io/otel/sdk/resource"
// 	tracesdk "go.opentelemetry.io/otel/sdk/trace"
// 	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
// 	"go.opentelemetry.io/otel/trace"
// )

// const (
// 	// DefaultTracingEnable is the default option to enable/disable the tracing.
// 	DefaultTracingEnable = false
// 	// DefaultTracingEndpoint is the default endpoint for the tracing.
// 	DefaultTracingEndpoint = "http://localhost:14268/api/traces"
// )

// // tracerProvider returns an OpenTelemetry TracerProvider configured to use
// // the Jaeger exporter that will send spans to the provided url. The returned
// // TracerProvider will also use a Resource configured with all the information
// // about the application.
// func tracerProvider(service string, collectorEndpoint string) (*tracesdk.TracerProvider, error) {
// 	var exp tracesdk.SpanExporter
// 	var err error
// 	if collectorEndpoint == "" {
// 		exp, err = stdouttrace.New(stdouttrace.WithPrettyPrint())
// 	} else {
// 		// Create the Jaeger exporter
// 		exp, err = jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(collectorEndpoint)))
// 		if err != nil {
// 			return nil, err
// 		}
// 	}

// 	bsp := tracesdk.NewBatchSpanProcessor(exp)
// 	tp := tracesdk.NewTracerProvider(
// 		// Always be sure to batch in production.
// 		// tracesdk.WithBatcher(exp),
// 		tracesdk.WithSampler(tracesdk.AlwaysSample()),
// 		tracesdk.WithSpanProcessor(bsp),
// 		// tracesdk.WithSyncer(exp),
// 		// Record information about this application in an Resource.
// 		tracesdk.WithResource(resource.NewWithAttributes(
// 			semconv.SchemaURL,
// 			semconv.ServiceNameKey.String(service),
// 			// attribute.String("environment", environment),
// 			// attribute.Int64("ID", id),
// 		)),
// 	)
// 	return tp, nil
// }

// // NewTracerProvider creates a new TracerProvider.
// func NewTracerProvider(service string) (trace.TracerProvider, func(context.Context), error) {
// 	// tracing enable
// 	tracingEnable := DefaultTracingEnable
// 	if envTracingEnable := os.Getenv("YOMO_TRACING_ENABLE"); envTracingEnable != "" {
// 		enable, err := strconv.ParseBool(envTracingEnable)
// 		if err == nil {
// 			tracingEnable = enable
// 		}
// 	}
// 	if !tracingEnable {
// 		return nil, func(context.Context) {}, errors.New("tracing disabled")
// 	}
// 	// tracer provider
// 	tracingEndpoint := DefaultTracingEndpoint
// 	if envTracingEndpoint := os.Getenv("YOMO_TRACING_ENDPOINT"); envTracingEndpoint != "" {
// 		tracingEndpoint = envTracingEndpoint
// 	}
// 	tp, err := tracerProvider(service, tracingEndpoint)
// 	if err != nil {
// 		return nil, func(context.Context) {}, err
// 	}
// 	// Cleanly shutdown and flush telemetry when the application exits.
// 	cleanup := func(ctx context.Context) {
// 		// Do not make the application hang when it is shutdown.
// 		ctx, cancel := context.WithTimeout(ctx, time.Second*5)
// 		defer cancel()
// 		if err := tp.Shutdown(ctx); err != nil {
// 			log.Fatal(err)
// 		}
// 	}
// 	// Register our TracerProvider as the global so any imported
// 	// instrumentation in the future will default to using it.
// 	otel.SetTracerProvider(tp)
// 	otel.SetTextMapPropagator(propagation.TraceContext{})
// 	// otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

// 	return tp, cleanup, nil

// }

// // NewTraceSpan creates a new span of OpenTelemetry from parent tracing.
// func NewTraceSpan(otelTraceID string, otelSpanID string, tracerName string, spanName string) (trace.Span, error) {
// 	return newTraceSpan(otel.GetTracerProvider(), otelTraceID, otelSpanID, tracerName, spanName, false)
// }

// // NewRemoteTraceSpan creates a new span of OpenTelemetry from remote parent tracing.
// func NewRemoteTraceSpan(otelTraceID string, otelSpanID string, tracerName string, spanName string) (trace.Span, error) {
// 	if otelTraceID == "" || otelSpanID == "" {
// 		return nil, errors.New("TraceID or SpanID is empty")
// 	}
// 	return newTraceSpan(otel.GetTracerProvider(), otelTraceID, otelSpanID, tracerName, spanName, true)
// }

// func newTraceSpan(tp trace.TracerProvider, otelTraceID string, otelSpanID string, tracerName string, spanName string, isremote bool) (trace.Span, error) {
// 	traceID, err := trace.TraceIDFromHex(otelTraceID)
// 	if err != nil {
// 		return nil, err
// 	}
// 	spanID, err := trace.SpanIDFromHex(otelSpanID)
// 	if err != nil {
// 		return nil, err
// 	}

// 	scc := trace.SpanContextConfig{
// 		TraceID: traceID,
// 		SpanID:  spanID,
// 	}
// 	ctx := context.Background()
// 	if isremote {
// 		ctx = trace.ContextWithRemoteSpanContext(ctx, trace.NewSpanContext(scc))
// 	} else {
// 		ctx = trace.ContextWithSpanContext(ctx, trace.NewSpanContext(scc))
// 	}
// 	tr := tp.Tracer(tracerName)
// 	_, span := tr.Start(ctx, spanName)
// 	return span, nil
// }

// // NewSpanFromData gets tje TraceID and SpanID from data and creates a new Span.
// func NewSpanFromData(data string, tracerName string, spanName string) trace.Span {
// 	// tracing
// 	var traceID, spanID string
// 	traceIDValue := gjson.Get(string(data), `metadatas.#(name=="TraceID").value`)
// 	if traceIDValue.Exists() {
// 		traceID = traceIDValue.String()
// 	}
// 	spanIDValue := gjson.Get(string(data), `metadatas.#(name=="SpanID").value`)
// 	if spanIDValue.Exists() {
// 		spanID = spanIDValue.String()
// 	}

// 	var span trace.Span
// 	if traceID != "" && spanID != "" {
// 		span, _ = NewRemoteTraceSpan(traceID, spanID, tracerName, spanName)
// 	}
// 	return span
// }
