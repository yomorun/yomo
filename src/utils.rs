use anyhow::{Result, bail};
use axum::http::HeaderMap;
use axum::response::Response;
use log::error;
use opentelemetry::trace::TraceContextExt;
use opentelemetry_sdk::trace::{IdGenerator, RandomIdGenerator};
use tracing::{Span, debug_span};
use tracing_opentelemetry::OpenTelemetrySpanExt;

use crate::auth::Auth;
use crate::metadata_mgr::MetadataMgr;
use crate::openai_http_mapping::openai_error_response;

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

fn header_value(headers: &HeaderMap, key: &str) -> String {
    headers
        .get(key)
        .and_then(|value| value.to_str().ok())
        .unwrap_or_default()
        .to_string()
}

pub(crate) fn credential_from_headers(headers: &HeaderMap) -> String {
    let credential = header_value(headers, "X-Credential");
    if !credential.trim().is_empty() {
        return credential;
    }
    let auth_header = header_value(headers, "Authorization");
    let bearer_prefix = "Bearer ";
    if let Some(token) = auth_header.strip_prefix(bearer_prefix) {
        return token.trim().to_string();
    }
    String::new()
}

pub(crate) async fn authenticate_and_metadata<A, M>(
    auth: &std::sync::Arc<dyn Auth<A>>,
    metadata_mgr: &std::sync::Arc<dyn MetadataMgr<A, M>>,
    headers: &HeaderMap,
    extension: &str,
    error_prefix: &str,
) -> Result<M, Response>
where
    A: Send + Sync + 'static,
    M: Send + Sync + 'static,
{
    let credential = credential_from_headers(headers);
    let auth_info = match auth.authenticate(&credential).await {
        Ok(info) => info,
        Err(err) => {
            error!("{} auth failed: {err}", error_prefix);
            return Err(openai_error_response(
                axum::http::StatusCode::UNAUTHORIZED,
                "unauthorized",
                Some("invalid_request_error"),
            ));
        }
    };

    let metadata = match metadata_mgr.new_from_extension(&auth_info, extension) {
        Ok(metadata) => metadata,
        Err(err) => {
            error!("metadata init failed: {err}");
            return Err(openai_error_response(
                axum::http::StatusCode::INTERNAL_SERVER_ERROR,
                "metadata error",
                None,
            ));
        }
    };

    Ok(metadata)
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
