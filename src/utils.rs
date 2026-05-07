use anyhow::{Result, bail};
use opentelemetry::trace::TraceContextExt;
use opentelemetry_sdk::trace::{IdGenerator, RandomIdGenerator};
use tracing::{Span, debug_span};
use tracing_opentelemetry::OpenTelemetrySpanExt;

pub fn sanitize_name(name: &str) -> Result<String> {
    let sanitized = name
        .chars()
        .map(|ch| {
            if ch.is_ascii_alphanumeric() || ch == '-' || ch == '_' {
                ch
            } else {
                '_'
            }
        })
        .collect::<String>();

    if sanitized.is_empty() {
        bail!("name is empty");
    }

    Ok(sanitized)
}

pub(crate) fn start_request_span(method: &str, route: &str) -> (Span, String) {
    let root_span = debug_span!("http.request", http.method = method, http.route = route);
    let trace_id = {
        let span_context = root_span.context().span().span_context().clone();
        if span_context.is_valid() {
            span_context.trace_id().to_string()
        } else {
            RandomIdGenerator::default().new_trace_id().to_string()
        }
    };
    (root_span, trace_id)
}
