use std::sync::Arc;
use std::{borrow::Cow, fmt, io::Read, pin::Pin, sync::Mutex};

use anyhow::{Context, anyhow};
use async_stream::try_stream;
use axum::Router;
use axum::body::{Body, Bytes};
use axum::extract::{Path, State};
use axum::http::{HeaderMap, Method, StatusCode, header};
use axum::response::{IntoResponse, Response};
use brotli::Decompressor;
use flate2::read::{GzDecoder, ZlibDecoder};
use futures_core::Stream;
use futures_util::{StreamExt, stream};
use log::{error, info, warn};
use serde::Serialize;
use serde_json::Value;
use tracing::{Instrument, Span};

use crate::agent_loop::{AgentLoopConfig, AgentLoopResult, run_agent_loop};
use crate::llm_stream_mapper::{DefaultStreamMapperSelector, StreamMapperSelector};
use crate::metadata_mgr::{MetadataMgr, MetadataMgrImpl};
use crate::openai_http_mapping::{
    map_chat_error, map_openai_response, map_usage_to_openai, openai_error_response,
    validate_openai_request,
};
use crate::openai_types::{ChatCompletionRequest, StreamOptions};
use crate::provider::{FinishReason, HttpProviderRequest, Provider, ProviderBody, ProviderError};
use crate::provider_registry::{ProviderRegistry, SelectionError};
use crate::serve_config::parse_generate_content_action;
use crate::tool_invoker::ToolInvoker;
use crate::tool_mgr::ToolMgr;
use crate::trace::{
    DefaultRequestSpanStarter, RequestSpanStarter, record_usage_attributes, set_http_span_status,
};
use crate::usage_handler::{EndpointUsage, UsageHandler};
use crate::utils::{MAX_LOG_BODY_BYTES, truncate_bytes_for_log, truncate_for_log};

const FIXED_CREATED_AT: i64 = 1_715_367_049;
const FIXED_OWNED_BY: &str = "system";

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum EndpointKind {
    ChatCompletions,
    Messages,
    Responses,
    Embeddings,
    Rerank,
    AudioSpeech,
    AudioTranscriptions,
    ImagesGenerations,
    ImagesEdits,
    ModelsGenerateContent,
    ModelsStreamGenerateContent,
}

#[derive(Clone)]
pub struct EndpointHandlerState<A, M> {
    pub provider_registry: Arc<ProviderRegistry<M>>,
    pub tool_mgr: Arc<dyn ToolMgr<A, M>>,
    pub tool_invoker: Arc<dyn ToolInvoker>,
    pub metadata_mgr: Arc<dyn MetadataMgr<A, M>>,
    pub request_span_starter: Arc<dyn RequestSpanStarter<M>>,
    pub agent_loop_config: AgentLoopConfig<M>,
    pub mapper_selector: Arc<dyn StreamMapperSelector>,
    pub usage_handler: Arc<dyn UsageHandler<M>>,
}

pub async fn handle_chat_completions<A, M>(
    State(state): State<EndpointHandlerState<A, M>>,
    headers: HeaderMap,
    body: Bytes,
) -> impl IntoResponse
where
    A: Send + Sync + 'static,
    M: fmt::Debug + Clone + Serialize + Send + Sync + 'static,
{
    let metadata = match state.metadata_mgr.new_from_http_headers(&headers) {
        Ok(metadata) => metadata,
        Err(err) => {
            let (root_span, _trace_id) =
                state
                    .request_span_starter
                    .start_request_span("POST", "/v1/chat/completions", None);
            root_span.record("http.request.body.size", body.len() as i64);
            error!("new metadata from headers: {err}");
            let message = err.to_string();
            set_http_span_status(&root_span, StatusCode::BAD_REQUEST, Some(&message));
            return openai_error_response(StatusCode::BAD_REQUEST, &message, None);
        }
    };
    let (root_span, trace_id) = state.request_span_starter.start_request_span(
        "POST",
        "/v1/chat/completions",
        Some(&metadata),
    );
    root_span.record("http.request.body.size", body.len() as i64);

    let root_span_for_error = root_span.clone();
    let instrument_span = root_span.clone();
    let response = match handle_chat_completions_inner::<A, M>(
        state,
        metadata.to_owned(),
        trace_id,
        body,
        root_span,
        headers.clone(),
    )
    .instrument(instrument_span)
    .await
    {
        Ok(response) => response,
        Err(err) => {
            error!("chat completion failed: {err} {:?}", metadata);
            let message = err.to_string();
            set_http_span_status(
                &root_span_for_error,
                StatusCode::INTERNAL_SERVER_ERROR,
                Some(&message),
            );
            openai_error_response(
                StatusCode::INTERNAL_SERVER_ERROR,
                "Internal Error, Please Try Again Later",
                None,
            )
        }
    };
    response
}

async fn handle_chat_completions_inner<A, M>(
    state: EndpointHandlerState<A, M>,
    metadata: M,
    trace_id: String,
    body: Bytes,
    root_span: Span,
    headers: HeaderMap,
) -> Result<Response, anyhow::Error>
where
    A: Send + Sync + 'static,
    M: fmt::Debug + Clone + Serialize + Send + Sync + 'static,
{
    let mut request: ChatCompletionRequest = match serde_json::from_slice(&body) {
        Ok(request) => request,
        Err(err) => {
            let request_body = truncate_bytes_for_log(&body);
            error!("chat request invalid json: {err} trace_id={trace_id} body={request_body}");
            let response = openai_error_response(
                StatusCode::BAD_REQUEST,
                &format!("invalid request body: {err}"),
                Some("invalid_request_error"),
            );
            set_http_span_status(&root_span, response.status(), Some(&err.to_string()));
            return Ok(response);
        }
    };

    let server_tools = state
        .tool_mgr
        .list_tools(&metadata)
        .await
        .map_err(|err| anyhow!("tool manager error: {err}"))?;

    if let Err(err) = state
        .agent_loop_config
        .request_hook
        .preprocess(&trace_id, &metadata, &mut request)
        .await
    {
        error!("request preprocess failed: {err}");
        let response = openai_error_response(
            StatusCode::BAD_REQUEST,
            &err.to_string(),
            Some("invalid_request_error"),
        );
        set_http_span_status(&root_span, response.status(), Some(&err.to_string()));
        return Ok(response);
    }

    let request_model_id = if request.model.trim().is_empty() || request.model.trim() == "auto" {
        None
    } else {
        Some(request.model.clone())
    };
    let provider_entry = match state.provider_registry.select_provider(
        EndpointKind::ChatCompletions,
        request_model_id.as_deref(),
        &metadata,
    ) {
        Ok(provider_entry) => provider_entry,
        Err(SelectionError::AccessDenied) => {
            let response = openai_error_response(
                StatusCode::FORBIDDEN,
                "access_denied",
                Some("access_denied"),
            );
            set_http_span_status(&root_span, response.status(), Some("access_denied"));
            return Ok(response);
        }
        Err(SelectionError::OutstandingBalance) => {
            let response = openai_error_response(
                StatusCode::PAYMENT_REQUIRED,
                "outstanding_balance",
                Some("outstanding_balance"),
            );
            set_http_span_status(&root_span, response.status(), Some("outstanding_balance"));
            return Ok(response);
        }
        Err(SelectionError::ModelRequired) => {
            let message = selection_model_required_message(&format!(
                "/v1{}",
                EndpointKind::ChatCompletions.as_path()
            ));
            let response = openai_error_response(
                StatusCode::BAD_REQUEST,
                &message,
                Some("invalid_request_error"),
            );
            set_http_span_status(&root_span, response.status(), Some(&message));
            return Ok(response);
        }
        Err(SelectionError::ModelNotSupported { model }) => {
            let message = selection_model_not_supported_message(
                &format!("/v1{}", EndpointKind::ChatCompletions.as_path()),
                &model,
            );
            let response = openai_error_response(
                StatusCode::BAD_REQUEST,
                &message,
                Some("invalid_request_error"),
            );
            set_http_span_status(&root_span, response.status(), Some(&message));
            return Ok(response);
        }
    };

    request.model = provider_entry.model_id.clone();
    let stream = request.stream.unwrap_or(false);
    if stream {
        match &mut request.stream_options {
            Some(options) => {
                options.include_usage = true;
            }
            None => {
                request.stream_options = Some(StreamOptions {
                    include_usage: true,
                    include_obfuscation: None,
                });
            }
        }
    }
    info!(
        "http.request.start; method=POST path=/v1/chat/completions model_id={} stream={} trace_id={} metadata={:?}",
        request_model_id
            .as_deref()
            .unwrap_or(&provider_entry.model_id),
        stream,
        trace_id,
        metadata
    );
    if let Err(message) = validate_openai_request(&request) {
        error!(
            "chat request invalid: model_id={}, error={} {:?}",
            request_model_id
                .as_deref()
                .unwrap_or(&provider_entry.model_id),
            message,
            metadata
        );
        let response = openai_error_response(
            StatusCode::BAD_REQUEST,
            &message,
            Some("invalid_request_error"),
        );
        set_http_span_status(&root_span, response.status(), Some(&message));
        return Ok(response);
    }

    let model_id = provider_entry.model_id.clone();
    let loop_result = run_agent_loop::<A, M>(
        provider_entry.provider,
        request,
        server_tools,
        state.tool_invoker.clone(),
        metadata.clone(),
        trace_id.clone(),
        provider_entry.label.clone(),
        state.agent_loop_config.clone(),
    )
    .await;

    match loop_result {
        Ok(AgentLoopResult::NonStream(response)) => {
            root_span.record(
                "finish_reason",
                tracing::field::display(finish_reason_to_str(&response.finish_reason)),
            );
            record_usage_attributes(&root_span, "usage", &response.usage);
            let usage = map_usage_to_openai(&response.usage);
            info!(
                "http.request.end; status_code=200 model_id={} prompt_tokens={} completion_tokens={} trace_id={} metadata={:?}",
                model_id, usage.prompt_tokens, usage.completion_tokens, trace_id, metadata
            );
            let mapped = map_openai_response(response);
            let payload = serde_json::to_vec(&mapped).context("serialize response")?;
            let response = Response::builder()
                .status(StatusCode::OK)
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(payload))
                .expect("build response");
            set_http_span_status(&root_span, response.status(), None);
            Ok(response)
        }
        Ok(AgentLoopResult::Stream { events }) => {
            let mut events = events;
            let Some(first_item) = events.next().await else {
                error!(
                    "http.request.end; status_code=500 model_id={} error=stream ended before first event trace_id={} metadata={:?}",
                    model_id, trace_id, metadata
                );
                let response = openai_error_response(
                    StatusCode::INTERNAL_SERVER_ERROR,
                    "Internal Error, Please Try Again Later",
                    None,
                );
                set_http_span_status(&root_span, response.status(), Some("internal_server_error"));
                return Ok(response);
            };
            let first_event = match first_item {
                Ok(event) => event,
                Err(err) => {
                    let status = provider_error_status(&err);
                    error!(
                        "http.request.end; status_code={} model_id={} error={} trace_id={} metadata={:?}",
                        status.as_u16(),
                        model_id,
                        err,
                        trace_id,
                        metadata
                    );
                    let status_message = trace_status_message_for_provider_error(&err, status);
                    let response = map_chat_error(err);
                    set_http_span_status(&root_span, status, Some(status_message.as_str()));
                    return Ok(response);
                }
            };
            let events = Box::pin(stream::once(async move { Ok(first_event) }).chain(events));
            let mapper = state.mapper_selector.select(&headers);
            let sse = mapper.map_stream(events, trace_id, model_id, root_span);
            let body = Body::from_stream(sse);
            Ok(Response::builder()
                .status(StatusCode::OK)
                .header(header::CONTENT_TYPE, "text/event-stream; charset=utf-8")
                .header(header::CACHE_CONTROL, "no-cache")
                .header(header::CONNECTION, "keep-alive")
                .body(body)
                .expect("build response"))
        }
        Err(err) => {
            let status = provider_error_status(&err);
            error!(
                "http.request.end; status_code={} model_id={} error={} trace_id={} metadata={:?}",
                status.as_u16(),
                model_id,
                err,
                trace_id,
                metadata
            );
            let status_message = trace_status_message_for_provider_error(&err, status);
            let response = map_chat_error(err);
            set_http_span_status(&root_span, status, Some(status_message.as_str()));
            Ok(response)
        }
    }
}

fn finish_reason_to_str(reason: &FinishReason) -> &'static str {
    match reason {
        FinishReason::Stop => "stop",
        FinishReason::Length => "length",
        FinishReason::ToolCalls => "tool_calls",
        FinishReason::ContentFilter => "content_filter",
        FinishReason::Other => "other",
    }
}

fn trace_status_message_for_provider_error(err: &ProviderError, status: StatusCode) -> String {
    if status == StatusCode::BAD_REQUEST {
        match err {
            ProviderError::Public { error, .. } => error.message.clone(),
            ProviderError::Internal { .. } => "internal_server_error".to_string(),
        }
    } else {
        "internal_server_error".to_string()
    }
}

fn provider_error_status(err: &ProviderError) -> StatusCode {
    match err {
        ProviderError::Public { status, .. } => *status,
        ProviderError::Internal { .. } => StatusCode::INTERNAL_SERVER_ERROR,
    }
}

fn selection_model_required_message(endpoint: &str) -> String {
    format!("model is required for {endpoint}")
}

fn selection_model_not_supported_message(endpoint: &str, model: &str) -> String {
    format!("model {model} is not supported for {endpoint}")
}

#[derive(Debug, Serialize)]
struct ModelListResponse {
    object: &'static str,
    data: Vec<ModelItem>,
}

#[derive(Debug, Serialize)]
struct ModelItem {
    id: String,
    object: &'static str,
    created: i64,
    owned_by: &'static str,
}

pub async fn handle_list_models<A, M>(
    State(state): State<EndpointHandlerState<A, M>>,
    headers: HeaderMap,
) -> impl IntoResponse
where
    A: Send + Sync + 'static,
    M: fmt::Debug + Clone + Serialize + Send + Sync + 'static,
{
    let metadata = match state.metadata_mgr.new_from_http_headers(&headers) {
        Ok(metadata) => metadata,
        Err(err) => {
            let (root_span, _trace_id) =
                state
                    .request_span_starter
                    .start_request_span("GET", "/v1/models", None);
            error!("new metadata from headers: {err}");
            let message = err.to_string();
            set_http_span_status(&root_span, StatusCode::BAD_REQUEST, Some(&message));
            return openai_error_response(StatusCode::BAD_REQUEST, &message, None);
        }
    };
    let (root_span, _trace_id) =
        state
            .request_span_starter
            .start_request_span("GET", "/v1/models", Some(&metadata));

    let mut unique = std::collections::HashMap::new();
    for model in state.provider_registry.list_models(&metadata) {
        unique.entry(model.to_ascii_lowercase()).or_insert(model);
    }

    let mut list: Vec<String> = unique.into_values().collect();
    list.sort_by_key(|model| model.to_ascii_lowercase());

    let data = list
        .into_iter()
        .map(|id| ModelItem {
            id,
            object: "model",
            created: FIXED_CREATED_AT,
            owned_by: FIXED_OWNED_BY,
        })
        .collect();
    set_http_span_status(&root_span, StatusCode::OK, None);
    axum::Json(ModelListResponse {
        object: "list",
        data,
    })
    .into_response()
}

#[cfg(test)]
mod chat_tests {
    use axum::http::StatusCode;

    use super::{
        selection_model_not_supported_message, selection_model_required_message,
        trace_status_message_for_provider_error,
    };
    use crate::openai_types::ErrorDetail;
    use crate::provider::ProviderError;

    #[test]
    fn trace_status_message_uses_public_message_for_bad_request() {
        let err = ProviderError::Public {
            status: StatusCode::BAD_REQUEST,
            error: ErrorDetail {
                message: "provider_bad_request".to_string(),
                r#type: "invalid_request_error".to_string(),
                code: None,
                param: None,
            },
        };

        assert_eq!(
            trace_status_message_for_provider_error(&err, StatusCode::BAD_REQUEST),
            "provider_bad_request"
        );
    }

    #[test]
    fn trace_status_message_uses_internal_for_non_bad_request() {
        let err = ProviderError::Public {
            status: StatusCode::UNPROCESSABLE_ENTITY,
            error: ErrorDetail {
                message: "provider_error".to_string(),
                r#type: "invalid_request_error".to_string(),
                code: None,
                param: None,
            },
        };

        assert_eq!(
            trace_status_message_for_provider_error(&err, StatusCode::UNPROCESSABLE_ENTITY),
            "internal_server_error"
        );
    }

    #[test]
    fn selection_model_required_message_includes_endpoint() {
        assert_eq!(
            selection_model_required_message("/v1/chat/completions"),
            "model is required for /v1/chat/completions"
        );
    }

    #[test]
    fn selection_model_not_supported_message_includes_endpoint() {
        assert_eq!(
            selection_model_not_supported_message("/v1/chat/completions", "gpt-x"),
            "model gpt-x is not supported for /v1/chat/completions"
        );
    }
}

async fn handle_static_endpoint<A, M>(
    endpoint_path: &'static str,
    endpoint: EndpointKind,
    state: EndpointHandlerState<A, M>,
    headers: HeaderMap,
    body: Bytes,
) -> Response
where
    A: Send + Sync + 'static,
    M: Send + Sync + fmt::Debug + Clone + 'static,
{
    handle_endpoint(endpoint_path, endpoint, None, None, state, headers, body).await
}

pub async fn handle_messages<A, M>(
    State(state): State<EndpointHandlerState<A, M>>,
    headers: HeaderMap,
    body: Bytes,
) -> impl IntoResponse
where
    A: Send + Sync + 'static,
    M: Send + Sync + fmt::Debug + Clone + 'static,
{
    handle_static_endpoint("/messages", EndpointKind::Messages, state, headers, body).await
}

pub async fn handle_responses<A, M>(
    State(state): State<EndpointHandlerState<A, M>>,
    headers: HeaderMap,
    body: Bytes,
) -> impl IntoResponse
where
    A: Send + Sync + 'static,
    M: Send + Sync + fmt::Debug + Clone + 'static,
{
    handle_static_endpoint("/responses", EndpointKind::Responses, state, headers, body).await
}

pub async fn handle_embeddings<A, M>(
    State(state): State<EndpointHandlerState<A, M>>,
    headers: HeaderMap,
    body: Bytes,
) -> impl IntoResponse
where
    A: Send + Sync + 'static,
    M: Send + Sync + fmt::Debug + Clone + 'static,
{
    handle_static_endpoint(
        "/embeddings",
        EndpointKind::Embeddings,
        state,
        headers,
        body,
    )
    .await
}

pub async fn handle_rerank<A, M>(
    State(state): State<EndpointHandlerState<A, M>>,
    headers: HeaderMap,
    body: Bytes,
) -> impl IntoResponse
where
    A: Send + Sync + 'static,
    M: Send + Sync + fmt::Debug + Clone + 'static,
{
    handle_static_endpoint("/rerank", EndpointKind::Rerank, state, headers, body).await
}

pub async fn handle_audio_speech<A, M>(
    State(state): State<EndpointHandlerState<A, M>>,
    headers: HeaderMap,
    body: Bytes,
) -> impl IntoResponse
where
    A: Send + Sync + 'static,
    M: Send + Sync + fmt::Debug + Clone + 'static,
{
    handle_static_endpoint(
        "/audio/speech",
        EndpointKind::AudioSpeech,
        state,
        headers,
        body,
    )
    .await
}

pub async fn handle_audio_transcriptions<A, M>(
    State(state): State<EndpointHandlerState<A, M>>,
    headers: HeaderMap,
    body: Bytes,
) -> impl IntoResponse
where
    A: Send + Sync + 'static,
    M: Send + Sync + fmt::Debug + Clone + 'static,
{
    handle_static_endpoint(
        "/audio/transcriptions",
        EndpointKind::AudioTranscriptions,
        state,
        headers,
        body,
    )
    .await
}

pub async fn handle_images_generations<A, M>(
    State(state): State<EndpointHandlerState<A, M>>,
    headers: HeaderMap,
    body: Bytes,
) -> impl IntoResponse
where
    A: Send + Sync + 'static,
    M: Send + Sync + fmt::Debug + Clone + 'static,
{
    handle_static_endpoint(
        "/images/generations",
        EndpointKind::ImagesGenerations,
        state,
        headers,
        body,
    )
    .await
}

pub async fn handle_images_edits<A, M>(
    State(state): State<EndpointHandlerState<A, M>>,
    headers: HeaderMap,
    body: Bytes,
) -> impl IntoResponse
where
    A: Send + Sync + 'static,
    M: Send + Sync + fmt::Debug + Clone + 'static,
{
    handle_static_endpoint(
        "/images/edits",
        EndpointKind::ImagesEdits,
        state,
        headers,
        body,
    )
    .await
}

pub async fn handle_generate_content<A, M>(
    Path(model_action): Path<String>,
    State(state): State<EndpointHandlerState<A, M>>,
    headers: HeaderMap,
    body: Bytes,
) -> impl IntoResponse
where
    A: Send + Sync + 'static,
    M: Send + Sync + fmt::Debug + Clone + 'static,
{
    let endpoint_path = format!("/models/{model_action}");
    let Some(action) = parse_generate_content_action(&endpoint_path) else {
        return openai_error_response(
            StatusCode::NOT_FOUND,
            "endpoint not found",
            Some("invalid_request_error"),
        );
    };
    let is_stream = action.endpoint == EndpointKind::ModelsStreamGenerateContent;
    handle_endpoint(
        &endpoint_path,
        action.endpoint,
        Some(action.model),
        Some(is_stream),
        state,
        headers,
        body,
    )
    .await
}

pub async fn handle_http_endpoint<A, M>(
    Path(path): Path<String>,
    State(state): State<EndpointHandlerState<A, M>>,
    headers: HeaderMap,
    body: Bytes,
) -> impl IntoResponse
where
    A: Send + Sync + 'static,
    M: Send + Sync + fmt::Debug + Clone + 'static,
{
    let endpoint_path = format!("/{}", path.trim_start_matches('/'));
    let resolved = match endpoint_path.as_str() {
        "/messages" => Some((EndpointKind::Messages, None, None)),
        "/responses" => Some((EndpointKind::Responses, None, None)),
        "/embeddings" => Some((EndpointKind::Embeddings, None, None)),
        "/rerank" => Some((EndpointKind::Rerank, None, None)),
        "/audio/speech" => Some((EndpointKind::AudioSpeech, None, None)),
        "/audio/transcriptions" => Some((EndpointKind::AudioTranscriptions, None, None)),
        "/images/generations" => Some((EndpointKind::ImagesGenerations, None, None)),
        "/images/edits" => Some((EndpointKind::ImagesEdits, None, None)),
        _ => parse_generate_content_action(&endpoint_path).map(|action| {
            let is_stream = action.endpoint == EndpointKind::ModelsStreamGenerateContent;
            (action.endpoint, Some(action.model), Some(is_stream))
        }),
    };

    let Some((endpoint, requested_model_from_path, forced_stream)) = resolved else {
        return openai_error_response(
            StatusCode::NOT_FOUND,
            "endpoint not found",
            Some("invalid_request_error"),
        );
    };

    handle_endpoint(
        &endpoint_path,
        endpoint,
        requested_model_from_path,
        forced_stream,
        state,
        headers,
        body,
    )
    .await
}

async fn handle_endpoint<A, M>(
    endpoint_path: &str,
    endpoint: EndpointKind,
    requested_model_from_path: Option<String>,
    forced_stream: Option<bool>,
    state: EndpointHandlerState<A, M>,
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
            error!("endpoint request parse failed: {err} trace_id={trace_id}");
            set_http_span_status(&root_span, StatusCode::BAD_REQUEST, Some(&err));
            return openai_error_response(
                StatusCode::BAD_REQUEST,
                &err,
                Some("invalid_request_error"),
            );
        }
    };
    let requested_model = requested_model_from_path.or(request_model);
    let is_stream = forced_stream.unwrap_or(is_stream);

    let provider_entry = match state.provider_registry.select_provider(
        endpoint,
        requested_model.as_deref(),
        &metadata,
    ) {
        Ok(provider_entry) => provider_entry,
        Err(SelectionError::AccessDenied) => {
            set_http_span_status(&root_span, StatusCode::FORBIDDEN, Some("access_denied"));
            return openai_error_response(
                StatusCode::FORBIDDEN,
                "access_denied",
                Some("access_denied"),
            );
        }
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
        Err(SelectionError::ModelRequired) => {
            let message = selection_model_required_message(&route);
            set_http_span_status(&root_span, StatusCode::BAD_REQUEST, Some(&message));
            return openai_error_response(
                StatusCode::BAD_REQUEST,
                &message,
                Some("invalid_request_error"),
            );
        }
        Err(SelectionError::ModelNotSupported { model }) => {
            let message = selection_model_not_supported_message(&route, &model);
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

    let provider_request = HttpProviderRequest {
        method: Method::POST,
        endpoint_path: endpoint_path.to_string(),
        headers: headers.clone(),
        body,
        is_stream,
        content_type,
    };

    let response = match provider_entry
        .provider
        .http(provider_request, &metadata)
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
    if !response.status.is_success() {
        state.provider_registry.notify_http_error(
            endpoint_path,
            &provider_entry.model_id,
            &metadata,
            response.status.as_u16(),
            format!("upstream returned status {}", response.status.as_u16()),
        );
    }
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
                let request_id =
                    resolve_request_id(provider.extract_request_id(&body_json), &trace_id);
                if let Some(usage_value) = provider
                    .extract_usage(&body_json)
                    .filter(|usage| !usage.is_null())
                {
                    let modified_usage = state
                        .usage_handler
                        .on_usage(
                            &provider_entry.model_id,
                            provider_entry.label.as_deref(),
                            &request_id,
                            &trace_id,
                            metadata.clone(),
                            EndpointUsage::from_endpoint_payload(endpoint_path, usage_value)
                                .expect("endpoint_api expected endpoint usage payload"),
                        )
                        .await;
                    record_usage_attributes(&root_span, "usage", &modified_usage);
                    let response_usage = modified_usage.into_payload(endpoint_path);
                    if provider.inject_usage(&mut body_json, response_usage) {
                        payload = serde_json::to_vec(&body_json)
                            .map(Bytes::from)
                            .unwrap_or(payload);
                    }
                }
            } else {
                let upstream_status_code = response.status.as_u16();
                let upstream_content_type = response
                    .headers
                    .get(header::CONTENT_TYPE)
                    .and_then(|value| value.to_str().ok())
                    .unwrap_or_default();
                let upstream_content_encoding = response
                    .headers
                    .get(header::CONTENT_ENCODING)
                    .and_then(|value| value.to_str().ok());
                warn!(
                    "endpoint got invalid json body for model_id {} status_code={} content_type={} content_encoding={} body={} trace_id={}",
                    provider_entry.model_id,
                    upstream_status_code,
                    upstream_content_type,
                    upstream_content_encoding.unwrap_or_default(),
                    body_preview_for_log(&payload, upstream_content_encoding),
                    trace_id
                );
            }
            let response = builder.body(Body::from(payload)).expect("build response");
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
        ProviderBody::Stream(stream) => builder
            .body(Body::from_stream(wrap_stream_with_usage(
                stream,
                root_span,
                provider,
                Arc::clone(&state.usage_handler),
                endpoint_path.to_string(),
                provider_entry.model_id.clone(),
                provider_entry.label.clone(),
                trace_id.clone(),
                metadata.clone(),
                format!("{:?}", metadata),
            )))
            .expect("build response"),
    };

    response
}

fn body_preview_for_log(payload: &Bytes, content_encoding: Option<&str>) -> String {
    let mut decode_failed = false;
    let decoded_payload = match decode_payload_for_log(payload, content_encoding) {
        Ok(decoded) => decoded,
        Err(_) => {
            decode_failed = true;
            Cow::Borrowed(payload.as_ref())
        }
    };

    if let Ok(decoded) = std::str::from_utf8(&decoded_payload) {
        return truncate_for_log(decoded);
    }

    let prefix_len = decoded_payload.len().min(24);
    let hex_prefix = decoded_payload
        .iter()
        .take(prefix_len)
        .map(|byte| format!("{byte:02x}"))
        .collect::<Vec<String>>()
        .join("");
    let encoding_hint = content_encoding
        .filter(|value| !value.is_empty())
        .map(|value| format!(", content_encoding={value}"))
        .unwrap_or_default();
    let decode_failed_hint = if decode_failed {
        ", decode_failed=true"
    } else {
        ""
    };

    format!(
        "<non-utf8 body: len={} first_bytes_hex={}{}{}>",
        decoded_payload.len(),
        hex_prefix,
        encoding_hint,
        decode_failed_hint
    )
}

fn decode_payload_for_log<'a>(
    payload: &'a Bytes,
    content_encoding: Option<&str>,
) -> Result<Cow<'a, [u8]>, std::io::Error> {
    let Some(encoding) = content_encoding
        .map(str::trim)
        .filter(|value| !value.is_empty())
    else {
        return Ok(Cow::Borrowed(payload.as_ref()));
    };

    let normalized = encoding
        .split(',')
        .next_back()
        .unwrap_or(encoding)
        .trim()
        .to_ascii_lowercase();

    match normalized.as_str() {
        "identity" => Ok(Cow::Borrowed(payload.as_ref())),
        "br" => {
            let mut decoded = Vec::new();
            let decoder = Decompressor::new(payload.as_ref(), 4096);
            let mut limited_decoder = decoder.take(MAX_LOG_BODY_BYTES as u64);
            limited_decoder.read_to_end(&mut decoded)?;
            Ok(Cow::Owned(decoded))
        }
        "gzip" | "x-gzip" => {
            let mut decoded = Vec::new();
            let decoder = GzDecoder::new(payload.as_ref());
            let mut limited_decoder = decoder.take(MAX_LOG_BODY_BYTES as u64);
            limited_decoder.read_to_end(&mut decoded)?;
            Ok(Cow::Owned(decoded))
        }
        "deflate" => {
            let mut decoded = Vec::new();
            let decoder = ZlibDecoder::new(payload.as_ref());
            let mut limited_decoder = decoder.take(MAX_LOG_BODY_BYTES as u64);
            limited_decoder.read_to_end(&mut decoded)?;
            Ok(Cow::Owned(decoded))
        }
        _ => Ok(Cow::Borrowed(payload.as_ref())),
    }
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
    provider: Arc<dyn Provider<M>>,
    usage_handler: Arc<dyn UsageHandler<M>>,
    endpoint: String,
    model_id: String,
    label: Option<String>,
    trace_id: String,
    metadata: M,
    metadata_debug: String,
) -> Pin<Box<dyn Stream<Item = Result<Bytes, std::io::Error>> + Send>>
where
    M: Clone + Send + Sync + 'static,
{
    Box::pin(try_stream! {
        futures_util::pin_mut!(stream);
        let mut finalizer = EndpointStreamFinalizer::new(
            root_span.clone(),
            trace_id.clone(),
            model_id.clone(),
            metadata_debug,
        );
        let mut text_buffer = String::new();
        let mut latest_usage: Option<EndpointUsage> = None;
        while let Some(item) = stream.next().await {
            let chunk = match item {
                Ok(chunk) => chunk,
                Err(err) => {
                    finalizer.set_failure(err.to_string());
                    Err(err)?
                }
            };
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
                    if let Some(usage_value) = provider.extract_usage(&value).filter(|usage| !usage.is_null())
                    {
                        let request_id = resolve_request_id(provider.extract_request_id(&value), &trace_id);
                        let modified_usage = usage_handler
                            .on_usage(
                                &model_id,
                                label.as_deref(),
                                &request_id,
                                &trace_id,
                                metadata.clone(),
                                EndpointUsage::from_endpoint_payload(&endpoint, usage_value)
                                    .expect("endpoint_api expected endpoint usage payload"),
                            )
                            .await;
                        let response_usage = modified_usage.clone().into_payload(&endpoint);
                        if provider.inject_usage(&mut value, response_usage) {
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

        if let Some(usage) = latest_usage {
            finalizer.set_usage(usage);
        }
    })
}

#[derive(Clone)]
struct EndpointStreamFinalizer {
    root_span: tracing::Span,
    trace_id: String,
    state: Arc<Mutex<EndpointStreamFinalState>>,
}

struct EndpointStreamFinalState {
    status_code: StatusCode,
    model_id: String,
    error: Option<String>,
    metadata_debug: String,
    usage: Option<EndpointUsage>,
}

impl EndpointStreamFinalizer {
    fn new(
        root_span: tracing::Span,
        trace_id: String,
        model_id: String,
        metadata_debug: String,
    ) -> Self {
        Self {
            root_span,
            trace_id,
            state: Arc::new(Mutex::new(EndpointStreamFinalState {
                status_code: StatusCode::OK,
                model_id,
                error: None,
                metadata_debug,
                usage: None,
            })),
        }
    }

    fn set_failure(&mut self, error: String) {
        if let Ok(mut state) = self.state.lock() {
            state.status_code = StatusCode::INTERNAL_SERVER_ERROR;
            state.error = Some(error);
        }
    }

    fn set_usage(&mut self, usage: EndpointUsage) {
        if let Ok(mut state) = self.state.lock() {
            state.usage = Some(usage);
        }
    }
}

impl Drop for EndpointStreamFinalizer {
    fn drop(&mut self) {
        let Ok(state) = self.state.lock() else {
            return;
        };
        if let Some(usage) = state.usage.as_ref() {
            record_usage_attributes(&self.root_span, "usage", usage);
        }
        set_http_span_status(&self.root_span, state.status_code, state.error.as_deref());
        if state.status_code == StatusCode::OK {
            info!(
                "http.request.end; status_code=200 model_id={} trace_id={} metadata={}",
                state.model_id, self.trace_id, state.metadata_debug
            );
        } else {
            error!(
                "http.request.end; status_code={} model_id={} error={} trace_id={} metadata={}",
                state.status_code.as_u16(),
                state.model_id,
                state.error.as_deref().unwrap_or("stream failed"),
                self.trace_id,
                state.metadata_debug
            );
        }
    }
}

async fn rewrite_sse_frame_usage<M>(
    provider: Arc<dyn Provider<M>>,
    frame: &str,
    usage_handler: Arc<dyn UsageHandler<M>>,
    endpoint: &str,
    model_id: &str,
    label: Option<&str>,
    trace_id: &str,
    metadata: M,
) -> (String, Option<EndpointUsage>)
where
    M: Clone + Send + Sync + 'static,
{
    if let Some(mut value) = parse_sse_data_json(frame) {
        if let Some(usage_value) = provider
            .extract_usage(&value)
            .filter(|usage| !usage.is_null())
        {
            let request_id = resolve_request_id(provider.extract_request_id(&value), trace_id);
            let modified_usage = usage_handler
                .on_usage(
                    model_id,
                    label,
                    &request_id,
                    trace_id,
                    metadata,
                    EndpointUsage::from_endpoint_payload(endpoint, usage_value)
                        .expect("endpoint_api expected endpoint usage payload"),
                )
                .await;
            let response_usage = modified_usage.clone().into_payload(endpoint);
            if provider.inject_usage(&mut value, response_usage) {
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

fn resolve_request_id(request_id: Option<String>, trace_id: &str) -> String {
    request_id
        .map(|id| id.trim().to_string())
        .filter(|id| !id.is_empty())
        .unwrap_or_else(|| trace_id.to_string())
}

#[cfg(test)]
mod http_tests {
    use super::{
        body_preview_for_log, decode_payload_for_log, parse_model_request_fields,
        resolve_request_id, selection_model_not_supported_message,
        selection_model_required_message,
    };
    use crate::utils::MAX_LOG_BODY_BYTES;
    use axum::body::Bytes;
    use brotli::CompressorReader;
    use flate2::Compression;
    use flate2::write::GzEncoder;
    use std::io::{Read, Write};

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

    /// Verifies request id resolver preserves non-empty extracted id values.
    #[test]
    fn resolve_request_id_prefers_extracted_value() {
        let request_id = resolve_request_id(Some("req_123".to_string()), "trace_123");

        assert_eq!(request_id, "req_123");
    }

    /// Verifies request id resolver falls back to trace id for missing or blank ids.
    #[test]
    fn resolve_request_id_falls_back_to_trace_id() {
        let missing = resolve_request_id(None, "trace_456");
        let blank = resolve_request_id(Some("   ".to_string()), "trace_789");

        assert_eq!(missing, "trace_456");
        assert_eq!(blank, "trace_789");
    }

    /// Verifies invalid upstream payload previews are UTF-8 safe and bounded.
    #[test]
    fn body_preview_for_log_handles_non_utf8_and_truncates() {
        let mut payload = vec![0xff, 0xfe];
        payload.extend(std::iter::repeat_n(b'a', MAX_LOG_BODY_BYTES + 128));
        let preview = body_preview_for_log(&Bytes::from(payload), Some("br"));

        assert!(preview.starts_with("<non-utf8 body: len="));
        assert!(preview.contains("first_bytes_hex=fffe"));
        assert!(preview.contains("content_encoding=br"));
    }

    /// Verifies UTF-8 payload preview keeps truncation behavior.
    #[test]
    fn body_preview_for_log_truncates_utf8_payload() {
        let payload = Bytes::from("a".repeat(MAX_LOG_BODY_BYTES + 64));
        let preview = body_preview_for_log(&payload, None);

        assert!(preview.contains("[truncated"));
    }

    /// Verifies Brotli-encoded error bodies are decoded for readable logs.
    #[test]
    fn body_preview_for_log_decodes_brotli_payload() {
        let raw = b"{\"error\":{\"message\":\"invalid_payload\"}}";
        let mut compressor = CompressorReader::new(&raw[..], 4096, 5, 22);
        let mut compressed = Vec::new();
        compressor
            .read_to_end(&mut compressed)
            .expect("compress test payload");

        let preview = body_preview_for_log(&Bytes::from(compressed), Some("br"));

        assert!(preview.contains("invalid_payload"));
        assert!(!preview.starts_with("<non-utf8 body"));
    }

    /// Verifies decoder applies the outermost content encoding first.
    #[test]
    fn decode_payload_for_log_uses_outermost_encoding() {
        let raw = b"outermost-encoding";
        let mut encoder = GzEncoder::new(Vec::new(), Compression::default());
        encoder.write_all(raw).expect("write gzip payload");
        let compressed = encoder.finish().expect("finish gzip payload");

        let payload = Bytes::from(compressed);
        let decoded =
            decode_payload_for_log(&payload, Some("identity, gzip")).expect("decode payload");

        assert_eq!(decoded.as_ref(), raw);
    }

    /// Verifies decode size is bounded to avoid decompression bomb logs.
    #[test]
    fn decode_payload_for_log_limits_decompressed_output() {
        let raw = "a".repeat(MAX_LOG_BODY_BYTES + 1024).into_bytes();
        let mut encoder = GzEncoder::new(Vec::new(), Compression::default());
        encoder
            .write_all(&raw)
            .expect("write oversized gzip payload");
        let compressed = encoder.finish().expect("finish oversized gzip payload");

        let payload = Bytes::from(compressed);
        let decoded =
            decode_payload_for_log(&payload, Some("gzip")).expect("decode payload with limit");

        assert_eq!(decoded.len(), MAX_LOG_BODY_BYTES);
    }

    /// Verifies selection errors include endpoint in model-required message.
    #[test]
    fn selection_model_required_message_includes_endpoint() {
        assert_eq!(
            selection_model_required_message("/v1/responses"),
            "model is required for /v1/responses"
        );
    }

    /// Verifies selection errors include endpoint in unsupported-model message.
    #[test]
    fn selection_model_not_supported_message_includes_endpoint() {
        assert_eq!(
            selection_model_not_supported_message("/v1/responses", "gpt-x"),
            "model gpt-x is not supported for /v1/responses"
        );
    }

    #[test]
    fn endpoint_routes_builds_explicit_paths() {
        let _ = super::endpoint_routes::<(), ()>();
    }
}

fn endpoint_routes<A, M>() -> Router<EndpointHandlerState<A, M>>
where
    A: Clone + Send + Sync + 'static,
    M: fmt::Debug + Clone + Serialize + Send + Sync + 'static,
{
    Router::new()
        .route("/models", axum::routing::get(handle_list_models::<A, M>))
        .route(
            "/chat/completions",
            axum::routing::post(handle_chat_completions::<A, M>),
        )
        .route(
            "/{*path}",
            axum::routing::post(handle_http_endpoint::<A, M>),
        )
}

pub async fn build_endpoint_api(
    tool_mgr: Arc<dyn ToolMgr<(), ()>>,
    provider_registry: ProviderRegistry<()>,
    tool_invoker: Arc<dyn ToolInvoker>,
    agent_loop_config: AgentLoopConfig<()>,
    usage_handler: Arc<dyn UsageHandler<()>>,
) -> anyhow::Result<Router> {
    let state = EndpointHandlerState {
        provider_registry: Arc::new(provider_registry),
        tool_mgr,
        tool_invoker,
        metadata_mgr: Arc::new(MetadataMgrImpl::new()),
        request_span_starter: Arc::new(DefaultRequestSpanStarter),
        agent_loop_config,
        mapper_selector: Arc::new(DefaultStreamMapperSelector::default()),
        usage_handler,
    };

    Ok(endpoint_routes::<(), ()>().with_state(state))
}

pub async fn build_endpoint_api_with_state<A, M>(
    state: EndpointHandlerState<A, M>,
) -> anyhow::Result<Router>
where
    A: Clone + Send + Sync + 'static,
    M: fmt::Debug + Clone + Serialize + Send + Sync + 'static,
{
    Ok(endpoint_routes::<A, M>().with_state(state))
}
