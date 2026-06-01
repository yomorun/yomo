use anyhow::{Context, Result};
use axum::http::StatusCode;
use opentelemetry::KeyValue;
use opentelemetry::global;
use opentelemetry::trace::{TraceContextExt, TracerProvider};
use opentelemetry_otlp::Protocol;
use opentelemetry_otlp::WithExportConfig;
use opentelemetry_sdk::propagation::TraceContextPropagator;
use opentelemetry_sdk::{
    Resource,
    trace::{self as sdktrace, IdGenerator, SdkTracerProvider},
};
use serde_json::Value;
use tracing::subscriber::set_global_default;
use tracing::{Span, field, info_span};
use tracing_opentelemetry::OpenTelemetrySpanExt;
use tracing_subscriber::filter::*;
use tracing_subscriber::prelude::*;

pub struct TraceGuard {
    enabled: bool,
    tracer_provider: Option<SdkTracerProvider>,
}

pub trait RequestSpanStarter<M>: Send + Sync {
    fn start_request_span(&self, method: &str, route: &str, metadata: Option<&M>)
    -> (Span, String);
}

#[derive(Default)]
pub struct DefaultRequestSpanStarter;

impl<M> RequestSpanStarter<M> for DefaultRequestSpanStarter
where
    M: serde::Serialize,
{
    fn start_request_span(
        &self,
        method: &str,
        route: &str,
        metadata: Option<&M>,
    ) -> (Span, String) {
        start_request_span(method, route, metadata)
    }
}

impl Drop for TraceGuard {
    fn drop(&mut self) {
        if self.enabled {
            if let Some(provider) = self.tracer_provider.take() {
                let _ = provider.shutdown();
            }
        }
    }
}

pub async fn init_tracing() -> Result<TraceGuard> {
    let filter = tracing_subscriber::EnvFilter::try_from_default_env()
        .unwrap_or_else(|_| tracing_subscriber::EnvFilter::new("info"));
    let fmt_layer = tracing_subscriber::fmt::layer()
        .with_writer(|| std::io::stdout())
        .with_filter(filter_fn(|meta| !meta.is_span()));

    let endpoint = match std::env::var("OTEL_EXPORTER_OTLP_ENDPOINT") {
        Ok(value) if !value.trim().is_empty() => value,
        _ => {
            let _ = set_global_default(tracing_subscriber::registry().with(filter).with(fmt_layer));
            return Ok(TraceGuard {
                enabled: false,
                tracer_provider: None,
            });
        }
    };
    let service_name = std::env::var("OTEL_SERVICE_NAME").unwrap_or_else(|_| "yomo".to_string());
    global::set_text_map_propagator(TraceContextPropagator::new());
    let exporter = opentelemetry_otlp::SpanExporter::builder()
        .with_http()
        .with_protocol(Protocol::HttpBinary)
        .with_endpoint(&endpoint)
        .build()
        .context("build otlp span exporter")?;
    let tracer_provider = SdkTracerProvider::builder()
        .with_resource(
            Resource::builder_empty()
                .with_attributes([KeyValue::new("service.name", service_name.clone())])
                .build(),
        )
        .with_batch_exporter(exporter)
        .build();
    let tracer = tracer_provider.tracer(service_name.clone());
    let otel_layer = tracing_opentelemetry::layer()
        .with_tracer(tracer)
        .with_tracked_inactivity(false)
        .with_location(false)
        .with_threads(false)
        .with_filter(filter_fn(|meta| meta.is_span()));
    let subscriber = tracing_subscriber::registry().with(filter).with(otel_layer);
    set_global_default(subscriber).context("set tracing subscriber")?;
    log::info!("tracing enabled: endpoint={endpoint}, service={service_name}");
    Ok(TraceGuard {
        enabled: true,
        tracer_provider: Some(tracer_provider),
    })
}

pub fn start_request_span<M>(method: &str, route: &str, _metadata: Option<&M>) -> (Span, String)
where
    M: serde::Serialize,
{
    let endpoint_name = if route.is_empty() { "/" } else { route };
    let root_span = info_span!(
        "http.request",
        otel.name = field::Empty,
        http.method = method,
        http.route = route,
        http.request.body.size = field::Empty,
        http.response.status_code = field::Empty,
        otel.status_code = field::Empty,
        otel.status_message = field::Empty,
        finish_reason = field::Empty,
    );
    root_span.record("otel.name", field::display(endpoint_name));

    let trace_id = {
        let span_context = root_span.context().span().span_context().clone();
        if span_context.is_valid() {
            span_context.trace_id().to_string()
        } else {
            sdktrace::RandomIdGenerator::default()
                .new_trace_id()
                .to_string()
        }
    };
    (root_span, trace_id)
}

pub(crate) fn set_http_span_status(span: &Span, status_code: StatusCode, message: Option<&str>) {
    span.record("http.response.status_code", i64::from(status_code.as_u16()));
    if status_code.is_client_error() || status_code.is_server_error() {
        let status_message = message
            .filter(|value| !value.trim().is_empty())
            .map(ToOwned::to_owned)
            .unwrap_or_else(|| {
                status_code
                    .canonical_reason()
                    .unwrap_or("request failed")
                    .to_string()
            });
        span.record("otel.status_code", "ERROR");
        span.record("otel.status_message", field::display(&status_message));
    } else {
        span.record("otel.status_code", "OK");
        span.record("otel.status_message", field::display(""));
    }
}

pub(crate) fn record_flattened_json_attributes(span: &Span, prefix: &str, value: &Value) {
    let mut attributes: Vec<(String, Value)> = Vec::new();
    collect_flattened_attributes(prefix.to_string(), value, &mut attributes);
    for (key, value) in attributes {
        match value {
            Value::Bool(v) => span.set_attribute(key, v),
            Value::Number(v) => {
                if let Some(i) = v.as_i64() {
                    if i != 0 {
                        span.set_attribute(key, i);
                    }
                } else if let Some(u) = v.as_u64() {
                    if u != 0 {
                        if let Ok(i) = i64::try_from(u) {
                            span.set_attribute(key, i);
                        } else {
                            span.set_attribute(key, u.to_string());
                        }
                    }
                } else if let Some(f) = v.as_f64() {
                    if f != 0.0 {
                        span.set_attribute(key, f);
                    }
                }
            }
            Value::String(v) => span.set_attribute(key, v),
            _ => {}
        }
    }
}

fn collect_flattened_attributes(path: String, value: &Value, out: &mut Vec<(String, Value)>) {
    match value {
        Value::Null => {}
        Value::Bool(v) => out.push((path, Value::Bool(*v))),
        Value::Number(v) => out.push((path, Value::Number(v.clone()))),
        Value::String(v) => out.push((path, Value::String(v.clone()))),
        Value::Array(items) => {
            for (index, item) in items.iter().enumerate() {
                collect_flattened_attributes(format!("{path}.{index}"), item, out);
            }
        }
        Value::Object(map) => {
            for (key, item) in map {
                collect_flattened_attributes(format!("{path}.{key}"), item, out);
            }
        }
    }
}
