package core

import (
	"github.com/yomorun/yomo/core/metadata"
	"github.com/yomorun/yomo/pkg/id"
	"github.com/yomorun/yomo/pkg/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
	"golang.org/x/exp/slog"
)

// NewMetadata returns metadata for yomo working.
func NewMetadata(sourceID, tid string, traceID string, spanID string, traced bool) metadata.M {
	return metadata.M{
		metadata.SourceIDKey: sourceID,
		metadata.TIDKey:      tid,
		metadata.TraceIDKey:  traceID,
		metadata.SpanIDKey:   spanID,
		metadata.TracedKey:   tracedString(traced),
	}
}

// GetTIDFromMetadata gets TID from metadata.
func GetTIDFromMetadata(m metadata.M) string {
	tid, _ := m.Get(metadata.TIDKey)
	return tid
}

// GetTracedFromMetadata gets traced from metadata.
func GetTracedFromMetadata(m metadata.M) bool {
	tracedString, _ := m.Get(metadata.TracedKey)
	return tracedString == "true"
}

// SetMetadataTarget sets target in metadata.
func SetMetadataTarget(m metadata.M, target string) {
	m.Set(metadata.TargetKey, target)
}

// InitialSourceMetadata generates initial source metadata with trace information.
// the span name typically corresponds to the source's name.
func InitialSourceMetadata(
	sourceID, tid string,
	spanName string,
	tp oteltrace.TracerProvider, logger *slog.Logger,
) (metadata.M, func()) {
	return initialMetadata(sourceID, tid, "Source", spanName, tp, logger)
}

// InitialSfnMetadata generates initial sfn metadata with trace information.
// the span name typically corresponds to the sfn's name.
func InitialSfnMetadata(
	sourceID, tid string,
	spanName string,
	tp oteltrace.TracerProvider, logger *slog.Logger,
) (metadata.M, func()) {
	return initialMetadata(sourceID, tid, "StreamFunction", spanName, tp, logger)
}

// initialMetadata builds a metadata with trace information.
// the tracer name is `Source` or `StreamFunction`.
// span name typically corresponds to the source's name or sfn's name.
func initialMetadata(
	sourceID, tid string,
	tracerName string,
	spanName string,
	tp oteltrace.TracerProvider, logger *slog.Logger,
) (metadata.M, func()) {
	var (
		traceID string
		spanID  string
		traced  bool
		endFn   = func() {}
	)
	if tp != nil {
		span, err := trace.NewSpan(tp, "Source", spanName, "", "")
		if err != nil {
			logger.Debug("trace error", "tracer_name", "Source", "span_name", spanName, "err", err)
		} else {
			endFn = func() { span.End() }
			traceID = span.SpanContext().TraceID().String()
			spanID = span.SpanContext().SpanID().String()
			traced = true
		}
	}
	if traceID == "" {
		logger.Debug("create new traceID", "tracer_name", "Source", "span_name", spanName, "trace_id", traceID)
		traceID = id.NewTraceID()
	}
	if spanID == "" {
		logger.Debug("create new spanID", "tracer_name", "Source", "span_name", spanName, "span_id", spanID)
		spanID = id.NewSpanID()
	}
	logger.Debug(
		"trace metadata",
		"tracer_name", "Source", "span_name", spanName,
		"trace_id", traceID, "span_id", spanID, "traced", traced,
	)
	md := NewMetadata(sourceID, id.New(), traceID, spanID, traced)

	return md, endFn
}

// ExtendTraceMetadata extends source metadata with trace information.
func ExtendTraceMetadata(
	md metadata.M,
	tracerName string, // the tracer name is `StreamFunction` or `Zipper`.
	spanName string, // the span name usually is the sfn name.
	tp oteltrace.TracerProvider, logger *slog.Logger,
) (metadata.M, func()) {
	var (
		traceID, _   = md.Get(metadata.TraceIDKey)
		spanID, _    = md.Get(metadata.SpanIDKey)
		parentTraced = GetTracedFromMetadata(md)
		endFn        = func() {}
	)
	traced := false
	if tp != nil {
		var span oteltrace.Span
		var err error
		// set parent span, if not traced, use empty string
		if parentTraced {
			span, err = trace.NewSpan(tp, string(tracerName), spanName, traceID, spanID)
		} else {
			span, err = trace.NewSpan(tp, string(tracerName), spanName, "", "")
		}
		if err != nil {
			logger.Debug("trace error", "tracer_name", tracerName, "span_name", spanName, "err", err)
		} else {
			endFn = func() { span.End() }
			traceID = span.SpanContext().TraceID().String()
			spanID = span.SpanContext().SpanID().String()
			traced = true
		}
	}
	if traceID == "" {
		logger.Debug("create new traceID", "tracer_name", tracerName, "span_name", spanName, "trace_id", traceID)
		traceID = id.NewTraceID()
	}
	if spanID == "" {
		logger.Debug("create new spanID", "tracer_name", tracerName, "span_name", spanName, "span_id", spanID)
		spanID = id.NewSpanID()
	}
	logger.Debug(
		"trace metadata",
		"tracer_name", tracerName, "span_name", spanName,
		"trace_id", traceID, "span_id", spanID, "traced", traced, "parent_traced", parentTraced,
	)

	if tracerName == "Zipper" {
		traced = traced || parentTraced
	}

	// reallocate metadata with new TraceID and SpanID
	md.Set(metadata.TraceIDKey, traceID)
	md.Set(metadata.SpanIDKey, spanID)
	md.Set(metadata.TracedKey, tracedString(traced))

	return md, endFn
}

// SfnTraceMetadata extends metadata for StreamFunction.
func SfnTraceMetadata(md metadata.M, sfnName string, tp oteltrace.TracerProvider, logger *slog.Logger) (metadata.M, func()) {
	return ExtendTraceMetadata(md, "StreamFunction", sfnName, tp, logger)
}

// ZipperTraceMetadata extends metadata for Zipper.
func ZipperTraceMetadata(md metadata.M, tp oteltrace.TracerProvider, logger *slog.Logger) (metadata.M, func()) {
	return ExtendTraceMetadata(md, "Zipper", "zipper endpoint", tp, logger)
}

func tracedString(traced bool) string {
	if traced {
		return "true"
	}
	return "false"
}
