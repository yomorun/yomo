use opentelemetry::KeyValue;
use opentelemetry::global;
use opentelemetry_otlp::WithExportConfig;
use opentelemetry_sdk::propagation::TraceContextPropagator;
use opentelemetry_sdk::runtime::Tokio;
use opentelemetry_sdk::{Resource, trace as sdktrace};
use anyhow::Context;
use tracing::subscriber::set_global_default;
use tracing_subscriber::EnvFilter;
use tracing_subscriber::prelude::*;


pub async fn init_tracing() -> Result<(), anyhow::Error> {
    let endpoint = match std::env::var("OTEL_EXPORTER_OTLP_ENDPOINT") {
        Ok(value) if !value.trim().is_empty() => value,
        _ => return Ok(()),
    };
    let service_name = std::env::var("OTEL_SERVICE_NAME")
        .unwrap_or_else(|_| "yomo".to_string());
    global::set_text_map_propagator(TraceContextPropagator::new());
    let exporter = opentelemetry_otlp::new_exporter()
        .http()
        .with_endpoint(&endpoint);
    let tracer =
        opentelemetry_otlp::new_pipeline()
            .tracing()
            .with_exporter(exporter)
            .with_trace_config(sdktrace::config().with_resource(Resource::new(vec![
                KeyValue::new("service.name", service_name.clone()),
            ])))
            .install_batch(Tokio)
            .context("init otlp tracing")?;
    let otel_layer = tracing_opentelemetry::layer()
        .with_tracer(tracer)
        .with_location(false)
        .with_threads(false);
    let subscriber = tracing_subscriber::registry()
        .with(EnvFilter::try_from_default_env().unwrap_or_else(|_| EnvFilter::new("info")))
        .with(otel_layer);
    set_global_default(subscriber).context("set tracing subscriber")?;
    log::info!("tracing enabled: endpoint={endpoint}, service={service_name}");
    Ok(())
}
