use std::{fmt, pin::Pin};

use async_stream::try_stream;
use axum::body::{Body, Bytes};
use axum::extract::{Path, State};
use axum::http::{HeaderMap, Method, StatusCode, header};
use axum::response::{IntoResponse, Response};
use futures_core::Stream;
use futures_util::{StreamExt, stream};
use log::{error, info};
use serde_json::Value;
use std::sync::Arc;
use tracing::Instrument;

use crate::metadata_mgr::MetadataMgr;
use crate::model_api_provider::{ProviderBody, ProviderRegistry, ProviderRequest, SelectionError};
use crate::openai_http_mapping::openai_error_response;
use crate::trace::{
    DefaultRequestSpanStarter, RequestSpanStarter, record_flattened_json_attributes,
    set_http_span_status,
};
use crate::usage_handler::{EndpointUsage, UsageHandler};

pub struct ModelApiHandlerState<A, M> {
    pub provider_registry: std::sync::Arc<ProviderRegistry<M>>,
    pub usage_handler: std::sync::Arc<dyn UsageHandler<M>>,
    pub metadata_mgr: std::sync::Arc<dyn MetadataMgr<A, M>>,
    pub request_span_starter: std::sync::Arc<dyn RequestSpanStarter<M>>,
}

impl<A, M> Clone for ModelApiHandlerState<A, M> {
    fn clone(&self) -> Self {
        Self {
            provider_registry: std::sync::Arc::clone(&self.provider_registry),
            usage_handler: std::sync::Arc::clone(&self.usage_handler),
            metadata_mgr: std::sync::Arc::clone(&self.metadata_mgr),
            request_span_starter: std::sync::Arc::clone(&self.request_span_starter),
        }
    }
}

#[derive(Clone, Copy)]
enum EndpointKind {
    Messages,
    Responses,
    GenerateContent,
    Embeddings,
    Rerank,
    AudioSpeech,
    AudioTranscriptions,
    Images,
}

pub async fn handle_model_api<A, M>(
    Path(path): Path<String>,
    State(state): State<ModelApiHandlerState<A, M>>,
    headers: HeaderMap,
    body: Bytes,
) -> impl IntoResponse
where
    A: Send + Sync + 'static,
    M: Send + Sync + fmt::Debug + Clone + 'static,
{
    let endpoint_path = format!("/{path}");
    let requested_model_from_path = parse_generate_content_model(&endpoint_path);
    let Some(kind) = resolve_endpoint_kind(&endpoint_path) else {
        return openai_error_response(
            StatusCode::NOT_FOUND,
            "endpoint not found",
            Some("invalid_request_error"),
        );
    };
    let selection_endpoint = if matches!(kind, EndpointKind::GenerateContent) {
        "/models/:generateContent"
    } else {
        endpoint_path.as_str()
    };
    handle_endpoint(
        &endpoint_path,
        selection_endpoint,
        requested_model_from_path,
        state,
        headers,
        body,
    )
    .await
}

async fn handle_endpoint<A, M>(
    endpoint_path: &str,
    selection_endpoint: &str,
    requested_model_from_path: Option<String>,
    state: ModelApiHandlerState<A, M>,
    headers: HeaderMap,
    body: Bytes,
) -> Response
where
    A: Send + Sync + 'static,
    M: fmt::Debug + Clone + Send + Sync + 'static,
{
    let route = format!("/v1{endpoint_path}");
    let metadata = match state.metadata_mgr.new_from_http_headers(&headers) {
        Ok(metadata) => metadata,
        Err(err) => {
            let (root_span, _trace_id) = state
                .request_span_starter
                .start_request_span("POST", &route, None);
            root_span.record("http.request.body.size", body.len() as i64);
            error!("new metadata from headers: {err}");
            let message = err.to_string();
            set_http_span_status(&root_span, StatusCode::BAD_REQUEST, Some(&message));
            return openai_error_response(StatusCode::BAD_REQUEST, &message, None);
        }
    };
    let (root_span, trace_id) =
        state
            .request_span_starter
            .start_request_span("POST", &route, Some(&metadata));
    root_span.record("http.request.body.size", body.len() as i64);

    let content_type = headers
        .get(header::CONTENT_TYPE)
        .and_then(|value| value.to_str().ok())
        .map(|value| value.to_string());
    let (request_model, is_stream) = match parse_model_request_fields(&content_type, &body).await {
        Ok(result) => result,
        Err(err) => {
            error!("model api request parse failed: {err} trace_id={trace_id}");
            set_http_span_status(&root_span, StatusCode::BAD_REQUEST, Some(&err));
            return openai_error_response(
                StatusCode::BAD_REQUEST,
                &err,
                Some("invalid_request_error"),
            );
        }
    };
    let requested_model = requested_model_from_path.or(request_model);

    let provider_entry = match state.provider_registry.select(
        selection_endpoint,
        requested_model.as_deref(),
        &metadata,
    ) {
        Ok(provider_entry) => provider_entry,
        Err(SelectionError::OutstandingBalance) => {
            set_http_span_status(
                &root_span,
                StatusCode::PAYMENT_REQUIRED,
                Some("outstanding_balance"),
            );
            return openai_error_response(
                StatusCode::PAYMENT_REQUIRED,
                "outstanding_balance",
                Some("outstanding_balance"),
            );
        }
        Err(SelectionError::ModelNotSupported) => {
            let model = requested_model.as_deref().unwrap_or("");
            let message = if model.is_empty() {
                "model is required".to_string()
            } else {
                format!("model {model} is not supported")
            };
            set_http_span_status(&root_span, StatusCode::BAD_REQUEST, Some(&message));
            return openai_error_response(
                StatusCode::BAD_REQUEST,
                &message,
                Some("invalid_request_error"),
            );
        }
    };

    info!(
        "http.request.start; method=POST path=/v1{} model_id={} stream={} trace_id={} metadata={:?}",
        endpoint_path, provider_entry.model_id, is_stream, trace_id, metadata
    );

    let provider_request = ProviderRequest {
        method: Method::POST,
        endpoint_path: endpoint_path.to_string(),
        headers: headers.clone(),
        body,
        is_stream,
        content_type,
    };

    let response = match provider_entry
        .provider
        .execute(provider_request)
        .instrument(root_span.clone())
        .await
    {
        Ok(response) => response,
        Err(err) => {
            error!(
                "http.request.end; status_code=500 model_id={} error={:?} trace_id={} metadata={:?}",
                provider_entry.model_id, err, trace_id, metadata
            );
            set_http_span_status(
                &root_span,
                StatusCode::INTERNAL_SERVER_ERROR,
                Some("internal_server_error"),
            );
            return Response::builder()
                .status(StatusCode::INTERNAL_SERVER_ERROR)
                .header(header::CONTENT_TYPE, "text/plain; charset=utf-8")
                .body(Body::from("internal server error"))
                .expect("build response");
        }
    };

    let mut builder = Response::builder().status(response.status);
    for (key, value) in response.headers.iter() {
        if key == header::CONTENT_LENGTH {
            continue;
        }
        builder = builder.header(key, value);
    }

    let provider = Arc::clone(&provider_entry.provider);
    let response = match response.body {
        ProviderBody::Full(payload) => {
            let mut payload = payload;
            if let Ok(mut body_json) = serde_json::from_slice::<Value>(&payload) {
                let request_id = provider
                    .extract_request_id_from_full(&body_json)
                    .unwrap_or_default();
                if let Some(usage_value) = provider
                    .extract_usage_from_full(&body_json)
                    .filter(|usage| !usage.is_null())
                {
                    let modified_usage = state
                        .usage_handler
                        .on_usage(
                            endpoint_path,
                            &provider_entry.model_id,
                            provider_entry.label.as_deref(),
                            &request_id,
                            &trace_id,
                            metadata.clone(),
                            EndpointUsage::from_endpoint_payload(endpoint_path, usage_value),
                        )
                        .await
                        .into_payload(endpoint_path);
                    record_flattened_json_attributes(&root_span, "usage", &modified_usage);
                    if provider.inject_usage_into_full(&mut body_json, modified_usage) {
                        payload = serde_json::to_vec(&body_json)
                            .map(Bytes::from)
                            .unwrap_or(payload);
                    }
                }
            }
            builder.body(Body::from(payload)).expect("build response")
        }
        ProviderBody::Stream(stream) => builder
            .body(Body::from_stream(wrap_stream_with_usage(
                stream,
                root_span.clone(),
                provider,
                Arc::clone(&state.usage_handler),
                endpoint_path.to_string(),
                provider_entry.model_id.clone(),
                provider_entry.label.clone(),
                trace_id.clone(),
                metadata.clone(),
            )))
            .expect("build response"),
    };

    info!(
        "http.request.end; status_code={} model_id={} trace_id={} metadata={:?}",
        response.status().as_u16(),
        provider_entry.model_id,
        trace_id,
        metadata
    );

    set_http_span_status(&root_span, response.status(), None);

    response
}

/// Parses request-level metadata from HTTP content type and body payload.
///
/// Returns the requested model identifier (if present) and whether streaming
/// mode is enabled. JSON requests read `model` and `stream`; multipart requests
/// read only `model` and always return `stream = false`.
async fn parse_model_request_fields(
    content_type: &Option<String>,
    body: &Bytes,
) -> Result<(Option<String>, bool), String> {
    if let Some(content_type) = content_type {
        if content_type.starts_with("application/json") {
            let value: Value =
                serde_json::from_slice(body).map_err(|err| format!("invalid json body: {err}"))?;
            let model = value
                .get("model")
                .and_then(|value| value.as_str())
                .map(|value| value.to_string());
            let stream = value
                .get("stream")
                .and_then(|value| value.as_bool())
                .unwrap_or(false);
            return Ok((model, stream));
        }
        if content_type.starts_with("multipart/form-data") {
            let model = parse_multipart_model(content_type, body).await?;
            return Ok((model, false));
        }
    }
    Ok((None, false))
}

async fn parse_multipart_model(content_type: &str, body: &Bytes) -> Result<Option<String>, String> {
    let boundary = parse_multipart_boundary(content_type)
        .ok_or_else(|| "multipart boundary is missing".to_string())?;
    let stream = stream::once(async move { Ok::<Bytes, multer::Error>(body.clone()) });
    let mut multipart = multer::Multipart::new(stream, boundary);
    while let Some(field) = multipart
        .next_field()
        .await
        .map_err(|err| format!("multipart error: {err}"))?
    {
        if field.name() == Some("model") {
            let value = field
                .text()
                .await
                .map_err(|err| format!("multipart field error: {err}"))?;
            return Ok(Some(value));
        }
    }
    Ok(None)
}

fn parse_multipart_boundary(content_type: &str) -> Option<String> {
    content_type.split(';').find_map(|part| {
        let part = part.trim();
        part.strip_prefix("boundary=")
            .map(|value| value.trim_matches('"').to_string())
    })
}

fn wrap_stream_with_usage<M>(
    stream: Pin<Box<dyn Stream<Item = Result<Bytes, std::io::Error>> + Send>>,
    root_span: tracing::Span,
    provider: Arc<dyn crate::model_api_provider::ModelApiProvider>,
    usage_handler: Arc<dyn UsageHandler<M>>,
    endpoint: String,
    model_id: String,
    label: Option<String>,
    trace_id: String,
    metadata: M,
) -> Pin<Box<dyn Stream<Item = Result<Bytes, std::io::Error>> + Send>>
where
    M: Clone + Send + Sync + 'static,
{
    Box::pin(try_stream! {
        futures_util::pin_mut!(stream);
        let mut text_buffer = String::new();
        let mut latest_usage: Option<Value> = None;
        while let Some(item) = stream.next().await {
            let chunk = item?;
            if let Ok(text) = std::str::from_utf8(&chunk) {
                text_buffer.push_str(text);
                while let Some(frame_end) = text_buffer.find("\n\n") {
                    let frame = text_buffer[..frame_end].to_string();
                    text_buffer.drain(..frame_end + 2);
                    let (output_frame, usage) = rewrite_sse_frame_usage(
                        Arc::clone(&provider),
                        &frame,
                        Arc::clone(&usage_handler),
                        &endpoint,
                        &model_id,
                        label.as_deref(),
                        &trace_id,
                        metadata.clone(),
                    )
                    .await;
                    if usage.is_some() {
                        latest_usage = usage;
                    }
                    yield Bytes::from(output_frame);
                }
            } else {
                yield chunk;
            }
        }

        if !text_buffer.trim().is_empty() {
            if text_buffer.trim_start().starts_with("data:") {
                let (output_frame, usage) = rewrite_sse_frame_usage(
                    Arc::clone(&provider),
                    text_buffer.trim_end_matches('\n'),
                    Arc::clone(&usage_handler),
                    &endpoint,
                    &model_id,
                    label.as_deref(),
                    &trace_id,
                    metadata.clone(),
                )
                .await;
                if usage.is_some() {
                    latest_usage = usage;
                }
                yield Bytes::from(output_frame);
            } else {
                if let Ok(mut value) = serde_json::from_str::<Value>(text_buffer.trim()) {
                    if let Some(usage_value) = provider
                        .extract_usage_from_stream_event(&value)
                        .filter(|usage| !usage.is_null())
                    {
                        let request_id = provider
                            .extract_request_id_from_stream_event(&value)
                            .unwrap_or_default();
                        let modified_usage = usage_handler
                            .on_usage(
                                &endpoint,
                                &model_id,
                                label.as_deref(),
                                &request_id,
                                &trace_id,
                                metadata.clone(),
                                EndpointUsage::from_endpoint_payload(&endpoint, usage_value),
                            )
                            .await
                            .into_payload(&endpoint);
                        if provider
                            .inject_usage_into_stream_event(&mut value, modified_usage.clone())
                        {
                            latest_usage = Some(modified_usage);
                            if let Ok(encoded) = serde_json::to_vec(&value) {
                                yield Bytes::from(encoded);
                            } else {
                                yield Bytes::from(text_buffer.clone());
                            }
                        } else {
                            yield Bytes::from(text_buffer.clone());
                        }
                    } else {
                        yield Bytes::from(text_buffer.clone());
                    }
                } else {
                    yield Bytes::from(text_buffer.clone());
                }
            }
        }

        if let Some(usage_value) = latest_usage {
            record_flattened_json_attributes(&root_span, "usage", &usage_value);
        }
    })
}

async fn rewrite_sse_frame_usage<M>(
    provider: Arc<dyn crate::model_api_provider::ModelApiProvider>,
    frame: &str,
    usage_handler: Arc<dyn UsageHandler<M>>,
    endpoint: &str,
    model_id: &str,
    label: Option<&str>,
    trace_id: &str,
    metadata: M,
) -> (String, Option<Value>)
where
    M: Clone + Send + Sync + 'static,
{
    if let Some(mut value) = parse_sse_data_json(frame) {
        if let Some(usage_value) = provider
            .extract_usage_from_stream_event(&value)
            .filter(|usage| !usage.is_null())
        {
            let request_id = provider
                .extract_request_id_from_stream_event(&value)
                .unwrap_or_default();
            let modified_usage = usage_handler
                .on_usage(
                    endpoint,
                    model_id,
                    label,
                    &request_id,
                    trace_id,
                    metadata,
                    EndpointUsage::from_endpoint_payload(endpoint, usage_value),
                )
                .await
                .into_payload(endpoint);
            if provider.inject_usage_into_stream_event(&mut value, modified_usage.clone()) {
                if let Ok(encoded) = serde_json::to_string(&value) {
                    return (
                        rebuild_sse_frame_with_data(frame, &encoded),
                        Some(modified_usage),
                    );
                }
            }
        }
    }
    (format!("{frame}\n\n"), None)
}

fn rebuild_sse_frame_with_data(frame: &str, json_payload: &str) -> String {
    let mut lines = Vec::new();
    for line in frame.lines() {
        if !line.starts_with("data:") {
            lines.push(line.to_string());
        }
    }
    lines.push(format!("data: {json_payload}"));
    format!("{}\n\n", lines.join("\n"))
}

fn parse_sse_data_json(frame: &str) -> Option<Value> {
    let mut payload = String::new();
    for line in frame.lines() {
        if let Some(data) = line.strip_prefix("data:") {
            if !payload.is_empty() {
                payload.push('\n');
            }
            payload.push_str(data.trim_start());
        }
    }
    let payload = payload.trim();
    if payload.is_empty() || payload == "[DONE]" {
        return None;
    }
    serde_json::from_str(payload).ok()
}

fn resolve_endpoint_kind(endpoint: &str) -> Option<EndpointKind> {
    match endpoint {
        "/messages" => Some(EndpointKind::Messages),
        "/responses" => Some(EndpointKind::Responses),
        "/embeddings" => Some(EndpointKind::Embeddings),
        "/rerank" => Some(EndpointKind::Rerank),
        "/audio/speech" => Some(EndpointKind::AudioSpeech),
        "/audio/transcriptions" => Some(EndpointKind::AudioTranscriptions),
        "/images/generations" => Some(EndpointKind::Images),
        "/images/edits" => Some(EndpointKind::Images),
        _ => {
            if parse_generate_content_model(endpoint).is_some() {
                Some(EndpointKind::GenerateContent)
            } else {
                None
            }
        }
    }
}

fn parse_generate_content_model(endpoint: &str) -> Option<String> {
    endpoint
        .strip_prefix("/models/")
        .and_then(|value| value.strip_suffix(":generateContent"))
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(str::to_string)
}

pub async fn build_model_api(
    provider_registry: ProviderRegistry<()>,
    usage_handler: std::sync::Arc<dyn UsageHandler<()>>,
) -> anyhow::Result<axum::Router> {
    let state = ModelApiHandlerState {
        provider_registry: std::sync::Arc::new(provider_registry),
        usage_handler,
        metadata_mgr: std::sync::Arc::new(crate::metadata_mgr::MetadataMgrImpl::new()),
        request_span_starter: std::sync::Arc::new(DefaultRequestSpanStarter),
    };

    let app = axum::Router::new()
        .route("/*path", axum::routing::post(handle_model_api::<(), ()>))
        .with_state(state);
    Ok(app)
}

#[cfg(test)]
mod tests {
    use super::parse_model_request_fields;
    use axum::body::Bytes;

    /// Verifies JSON requests return both model and stream metadata.
    #[tokio::test]
    async fn parse_model_request_fields_reads_json_model_and_stream() {
        let content_type = Some("application/json".to_string());
        let body = Bytes::from_static(br#"{"model":"gpt-4o","stream":true}"#);

        let parsed = parse_model_request_fields(&content_type, &body).await;

        assert_eq!(parsed.unwrap(), (Some("gpt-4o".to_string()), true));
    }

    /// Verifies JSON requests without optional fields fall back to defaults.
    #[tokio::test]
    async fn parse_model_request_fields_defaults_missing_json_fields() {
        let content_type = Some("application/json".to_string());
        let body = Bytes::from_static(br#"{}"#);

        let parsed = parse_model_request_fields(&content_type, &body).await;

        assert_eq!(parsed.unwrap(), (None, false));
    }

    /// Verifies malformed JSON bodies are rejected with a parse error.
    #[tokio::test]
    async fn parse_model_request_fields_rejects_invalid_json_body() {
        let content_type = Some("application/json".to_string());
        let body = Bytes::from_static(br#"{"model":"gpt-4o""#);

        let parsed = parse_model_request_fields(&content_type, &body).await;

        assert!(parsed.is_err());
        assert!(
            parsed
                .err()
                .unwrap_or_default()
                .starts_with("invalid json body:")
        );
    }

    /// Verifies multipart requests extract the model field value.
    #[tokio::test]
    async fn parse_model_request_fields_reads_multipart_model() {
        let boundary = "test-boundary";
        let content_type = Some(format!("multipart/form-data; boundary={boundary}"));
        let body = Bytes::from(format!(
            "--{boundary}\r\nContent-Disposition: form-data; name=\"model\"\r\n\r\ngpt-4.1\r\n--{boundary}--\r\n"
        ));

        let parsed = parse_model_request_fields(&content_type, &body).await;

        assert_eq!(parsed.unwrap(), (Some("gpt-4.1".to_string()), false));
    }

    /// Verifies multipart requests without a boundary return a clear error.
    #[tokio::test]
    async fn parse_model_request_fields_rejects_multipart_without_boundary() {
        let content_type = Some("multipart/form-data".to_string());
        let body = Bytes::from_static(b"ignored");

        let parsed = parse_model_request_fields(&content_type, &body).await;

        assert_eq!(parsed, Err("multipart boundary is missing".to_string()));
    }

    /// Verifies unsupported or missing content types fall back to defaults.
    #[tokio::test]
    async fn parse_model_request_fields_defaults_for_unknown_content_type() {
        let content_type = Some("text/plain".to_string());
        let body = Bytes::from_static(b"hello");

        let parsed = parse_model_request_fields(&content_type, &body).await;

        assert_eq!(parsed.unwrap(), (None, false));
    }
}
