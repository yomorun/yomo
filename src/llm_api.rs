use std::borrow::Cow;
use std::fmt;
use std::io::{Error as IoError, Read};
use std::pin::Pin;
use std::sync::Arc;

use anyhow::{Context, anyhow};
use axum::Router;
use axum::body::{Body, Bytes};
use axum::extract::{Path, State};
use axum::http::{HeaderMap, StatusCode, header};
use axum::response::{IntoResponse, Response};
use brotli::Decompressor;
use flate2::read::{GzDecoder, ZlibDecoder};
use futures_core::Stream;
use futures_util::{StreamExt, stream};
use log::{debug, error, info, warn};
use multer::Error as MultipartError;
use multer::Multipart;
use serde::Serialize;
use serde_json::Value;
use tracing::{Instrument, Span};

use crate::agent_loop::{AgentLoopConfig, AgentLoopResult, run_agent_loop};
use crate::api;
use crate::api::hosted_tools::strip_hosted_tools;
use crate::llm_stream_mapper::{DefaultStreamMapperSelector, StreamMapperSelector};
use crate::metadata_mgr::{MetadataMgr, MetadataMgrImpl};
use crate::openai_http_mapping::{
    INTERNAL_SERVER_ERROR_MESSAGE, empty_usage, map_usage_to_openai, openai_error_response,
    validate_unified_request,
};
use crate::openai_types::ErrorDetail;
use crate::provider::{
    Provider, ProviderError, UnifiedEvent, UnifiedEventKind, UnifiedRequest, UnifiedResponse,
    UnifiedResponseError,
};
use crate::provider_registry::{ProviderRegistry, SelectionError};
use crate::serve_config::{EndpointKind, ResponseModelMode, parse_generate_content_model};
use crate::tool_api::redacted_http_headers;
use crate::tool_invoker::ToolInvoker;
use crate::tool_mgr::ToolMgr;
use crate::trace::{DefaultRequestSpanStarter, RequestSpanStarter};
use crate::trace::{record_usage_attributes, set_http_span_status};
use crate::usage_handler::UsageHandler;

/// Request id fallback (same semantics as model_api): use trace_id when empty.
fn resolve_passthrough_request_id(request_id: Option<String>, trace_id: &str) -> String {
    request_id
        .map(|id| id.trim().to_string())
        .filter(|id| !id.is_empty())
        .unwrap_or_else(|| trace_id.to_string())
}
use crate::utils::{
    MAX_LOG_BODY_BYTES, json_body_for_log, truncate_bytes_for_log, truncate_for_log,
};

#[derive(Clone)]
pub struct LlmHandlerState<A, M> {
    pub provider_registry: Arc<dyn ProviderRegistry<M>>,
    pub tool_mgr: Arc<dyn ToolMgr<A, M>>,
    pub tool_invoker: Arc<dyn ToolInvoker>,
    pub metadata_mgr: Arc<dyn MetadataMgr<A, M>>,
    pub request_span_starter: Arc<dyn RequestSpanStarter<M>>,
    pub agent_loop_config: AgentLoopConfig<M>,
    pub mapper_selector: Arc<dyn StreamMapperSelector>,
    pub error_response_policy: Arc<dyn LlmErrorResponsePolicy<M>>,
}

/// Decides how provider errors are converted to chat-completions HTTP responses.
pub trait LlmErrorResponsePolicy<M>: Send + Sync {
    fn handle_provider_error(
        &self,
        endpoint: EndpointKind,
        model_id: &str,
        metadata: &M,
        err: ProviderError,
    ) -> Response;
}

pub struct DefaultLlmErrorResponsePolicy;

impl<M> LlmErrorResponsePolicy<M> for DefaultLlmErrorResponsePolicy {
    fn handle_provider_error(
        &self,
        _endpoint: EndpointKind,
        _model_id: &str,
        _metadata: &M,
        err: ProviderError,
    ) -> Response {
        if matches!(&err, ProviderError::Public { status, .. } if *status == StatusCode::BAD_REQUEST)
        {
            return map_public_provider_error(err);
        }
        openai_error_response(
            StatusCode::INTERNAL_SERVER_ERROR,
            INTERNAL_SERVER_ERROR_MESSAGE,
            Some("internal_error"),
        )
    }
}

fn map_public_provider_error(err: ProviderError) -> Response {
    let ProviderError::Public { status, error } = err else {
        return openai_error_response(
            StatusCode::INTERNAL_SERVER_ERROR,
            INTERNAL_SERVER_ERROR_MESSAGE,
            Some("internal_error"),
        );
    };
    let response = crate::openai_types::ErrorResponse { error };
    let payload = serde_json::to_vec(&response).unwrap_or_else(|_| b"{}".to_vec());
    Response::builder()
        .status(status)
        .header(header::CONTENT_TYPE, "application/json")
        .body(Body::from(payload))
        .expect("build error response")
}

/// Execution engine state shared by the three client APIs.
pub struct EngineState<A, M> {
    pub tool_mgr: Arc<dyn ToolMgr<A, M>>,
    pub tool_invoker: Arc<dyn ToolInvoker>,
    pub agent_loop_config: AgentLoopConfig<M>,
    pub response_model_mode: ResponseModelMode,
}

/// The execution engine shared by the three client APIs: everything except parse/serialize lives here.
///
/// Server tools, RequestHook.preprocess, request rewriting (stream_options), request validation,
/// the agent loop, and response_model applied once at the IR exit.
/// Validation / preprocess failures uniformly return `Public{400, invalid_request_error}`,
/// byte-identical to the existing `openai_error_response` shape.
pub async fn execute<A, M>(
    provider: Arc<dyn Provider<M>>,
    model_id: String,
    label: Option<String>,
    mut request: UnifiedRequest,
    state: &EngineState<A, M>,
    metadata: M,
    trace_id: String,
) -> Result<AgentLoopResult, ProviderError>
where
    A: Send + Sync + 'static,
    M: fmt::Debug + Clone + serde::Serialize + Send + Sync + 'static,
{
    let bad_request_error = |message: String| ProviderError::Public {
        status: StatusCode::BAD_REQUEST,
        error: ErrorDetail {
            message,
            r#type: "invalid_request_error".to_string(),
            code: None,
            param: None,
        },
    };

    let server_tools = state
        .tool_mgr
        .list_tools(&metadata)
        .await
        .map_err(|err| ProviderError::internal(format!("tool manager error: {err}")))?;

    if let Err(err) = state
        .agent_loop_config
        .request_hook
        .preprocess(&trace_id, &metadata, &mut request)
        .await
    {
        return Err(bad_request_error(err.to_string()));
    }

    request.model = model_id.clone();
    let stream = request.controls.stream.unwrap_or(false);
    if stream {
        let options = request
            .controls
            .stream_options
            .get_or_insert_with(|| serde_json::json!({}));
        options["include_usage"] = Value::Bool(true);
    }
    if let Err(message) = validate_unified_request(&request) {
        return Err(bad_request_error(message));
    }
    // Validate client-selected capabilities before returning a lazy stream. The
    // loop repeats this check after server tools have been injected each round.
    if let Some(binding) = provider.binding() {
        crate::provider::adapt_bound_request(&mut request, binding)?;
    }

    let loop_result = run_agent_loop::<A, M>(
        provider,
        request,
        server_tools,
        state.tool_invoker.clone(),
        metadata,
        trace_id,
        label,
        state.agent_loop_config.clone(),
    )
    .await;

    match loop_result {
        Ok(AgentLoopResult::NonStream(mut response)) => {
            // Apply response_model once at the IR exit; all three client APIs inherit it automatically.
            response.model =
                response_model_for_mode(&state.response_model_mode, &model_id, response.model);
            Ok(AgentLoopResult::NonStream(response))
        }
        Ok(AgentLoopResult::Stream { events }) => {
            let mode = state.response_model_mode.clone();
            let routed_model = model_id;
            let events = events.map(move |item| {
                item.map(|mut event| {
                    apply_response_model_to_event(&mut event, &mode, &routed_model);
                    event
                })
            });
            Ok(AgentLoopResult::Stream {
                events: Box::pin(events),
            })
        }
        Err(err) => Err(err),
    }
}

pub async fn handle_chat_completions<A, M>(
    State(state): State<LlmHandlerState<A, M>>,
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
                INTERNAL_SERVER_ERROR_MESSAGE,
                None,
            )
        }
    };
    response
}

async fn handle_chat_completions_inner<A, M>(
    state: LlmHandlerState<A, M>,
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
    debug!(
        "http.request.headers; trace_id={} headers={:?}",
        trace_id,
        redacted_http_headers(&headers)
    );
    debug!(
        "http.request.body; trace_id={} body={}",
        trace_id,
        json_body_for_log(&body)
    );
    let bad_request_response = |message: &str| {
        log_bad_request(EndpointKind::ChatCompletions, message, &trace_id, &metadata);
        openai_error_response(
            StatusCode::BAD_REQUEST,
            message,
            Some("invalid_request_error"),
        )
    };
    let request: UnifiedRequest = match api::chat_completions::parse_request(&body) {
        Ok(request) => request,
        Err(err) => {
            let request_body = truncate_bytes_for_log(&body);
            error!(
                "chat request invalid json: {} status_code=400 trace_id={trace_id} body={request_body}",
                err.message
            );
            let response =
                openai_error_response(err.status(), &err.message, Some("invalid_request_error"));
            set_http_span_status(&root_span, response.status(), Some(&err.message));
            return Ok(response);
        }
    };

    let request_model_id = if request.model.trim().is_empty() || request.model.trim() == "auto" {
        None
    } else {
        Some(request.model.clone())
    };
    let provider_entry = match state.provider_registry.select_chat(
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
            let response = bad_request_response(&message);
            set_http_span_status(&root_span, response.status(), Some(&message));
            return Ok(response);
        }
        Err(SelectionError::ModelNotSupported { model }) => {
            let message = selection_model_not_supported_message(&model);
            let response = bad_request_response(&message);
            set_http_span_status(&root_span, response.status(), Some(&message));
            return Ok(response);
        }
        Err(SelectionError::ModelEndpointNotSupported { model, .. }) => {
            let message = selection_model_endpoint_not_supported_message(
                &format!("/v1{}", EndpointKind::ChatCompletions.as_path()),
                &model,
            );
            let response = bad_request_response(&message);
            set_http_span_status(&root_span, response.status(), Some(&message));
            return Ok(response);
        }
    };

    let stream = request.controls.stream.unwrap_or(false);
    info!(
        "http.request.start; method=POST path=/v1/chat/completions model_id={} stream={} trace_id={} metadata={:?}",
        request_model_id
            .as_deref()
            .unwrap_or(&provider_entry.model_id),
        stream,
        trace_id,
        metadata
    );

    let model_id = provider_entry.model_id.clone();
    let engine_state = EngineState {
        tool_mgr: state.tool_mgr.clone(),
        tool_invoker: state.tool_invoker.clone(),
        agent_loop_config: state.agent_loop_config.clone(),
        response_model_mode: provider_entry.response_model_mode.clone(),
    };
    let loop_result = execute::<A, M>(
        provider_entry.provider.clone(),
        model_id.clone(),
        provider_entry.label.clone(),
        request,
        &engine_state,
        metadata.clone(),
        trace_id.clone(),
    )
    .await;

    match loop_result {
        Ok(AgentLoopResult::NonStream(response)) => {
            root_span.record(
                "finish_reason",
                tracing::field::display(finish_reason_to_str(&response)),
            );
            let raw_usage = response.usage.clone().unwrap_or_else(empty_usage);
            record_usage_attributes(&root_span, "usage", &raw_usage);
            let usage = map_usage_to_openai(&raw_usage);
            info!(
                "http.request.end; status_code=200 model_id={} prompt_tokens={} completion_tokens={} trace_id={} metadata={:?}",
                model_id, usage.prompt_tokens, usage.completion_tokens, trace_id, metadata
            );
            let payload = api::chat_completions::serialize_non_stream(response)
                .context("serialize response")?;
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
                    INTERNAL_SERVER_ERROR_MESSAGE,
                    None,
                );
                set_http_span_status(&root_span, response.status(), Some("internal_server_error"));
                return Ok(response);
            };
            let first_event = match first_item {
                Ok(event) => event,
                Err(err) => {
                    let upstream_status = provider_error_status(&err);
                    let error_message = err.to_string();
                    let response = state.error_response_policy.handle_provider_error(
                        EndpointKind::ChatCompletions,
                        &model_id,
                        &metadata,
                        err,
                    );
                    let final_status = response.status();
                    error!(
                        "http.request.end; status_code={} upstream_status_code={} model_id={} error={} trace_id={} metadata={:?}",
                        final_status.as_u16(),
                        upstream_status.as_u16(),
                        model_id,
                        error_message,
                        trace_id,
                        metadata
                    );
                    set_http_span_status(&root_span, response.status(), Some("provider_error"));
                    return Ok(response);
                }
            };
            let events = Box::pin(stream::once(async move { Ok(first_event) }).chain(events));
            let mapper = state.mapper_selector.select(&headers);
            let response_model_mode = provider_entry.response_model_mode.clone();
            let sse = mapper.map_stream(events, trace_id, model_id, response_model_mode, root_span);
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
            let upstream_status = provider_error_status(&err);
            let error_message = err.to_string();
            let response = state.error_response_policy.handle_provider_error(
                EndpointKind::ChatCompletions,
                &model_id,
                &metadata,
                err,
            );
            let final_status = response.status();
            error!(
                "http.request.end; status_code={} upstream_status_code={} model_id={} error={} trace_id={} metadata={:?}",
                final_status.as_u16(),
                upstream_status.as_u16(),
                model_id,
                error_message,
                trace_id,
                metadata
            );
            set_http_span_status(&root_span, response.status(), Some("provider_error"));
            Ok(response)
        }
    }
}

/// `/v1/messages` API (Anthropic): parse -> select -> execute -> serialize.
pub async fn handle_messages<A, M>(
    State(state): State<LlmHandlerState<A, M>>,
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
                    .start_request_span("POST", "/v1/messages", None);
            root_span.record("http.request.body.size", body.len() as i64);
            error!("new metadata from headers: {err}");
            let message = err.to_string();
            set_http_span_status(&root_span, StatusCode::BAD_REQUEST, Some(&message));
            return crate::api::messages::error_response(
                StatusCode::BAD_REQUEST,
                "invalid_request_error",
                &message,
            );
        }
    };
    let (root_span, trace_id) =
        state
            .request_span_starter
            .start_request_span("POST", "/v1/messages", Some(&metadata));
    root_span.record("http.request.body.size", body.len() as i64);

    let root_span_for_error = root_span.clone();
    let instrument_span = root_span.clone();
    let response =
        handle_messages_inner::<A, M>(state, metadata.to_owned(), trace_id, body, root_span)
            .instrument(instrument_span)
            .await;
    match response {
        Ok(response) => response,
        Err(err) => {
            error!("messages completion failed: {err} {:?}", metadata);
            let message = err.to_string();
            set_http_span_status(
                &root_span_for_error,
                StatusCode::INTERNAL_SERVER_ERROR,
                Some(&message),
            );
            crate::api::messages::error_response(
                StatusCode::INTERNAL_SERVER_ERROR,
                "api_error",
                INTERNAL_SERVER_ERROR_MESSAGE,
            )
        }
    }
}

async fn handle_messages_inner<A, M>(
    state: LlmHandlerState<A, M>,
    metadata: M,
    trace_id: String,
    body: Bytes,
    root_span: Span,
) -> Result<Response, anyhow::Error>
where
    A: Send + Sync + 'static,
    M: fmt::Debug + Clone + Serialize + Send + Sync + 'static,
{
    use crate::api::messages;

    let bad_request_response = |message: String, error_type: &str| {
        log_bad_request(EndpointKind::Messages, &message, &trace_id, &metadata);
        messages::error_response(StatusCode::BAD_REQUEST, error_type, &message)
    };

    // Strip upstream-executed tools before parsing, so hosted tool types never
    // reach the unified request; malformed bodies are rejected instead of forwarded.
    let (body, stripped_tools) = match strip_hosted_tools(EndpointKind::Messages, body) {
        Ok((body, removed)) => (body, removed),
        Err(message) => {
            let response = bad_request_response(message, "invalid_request_error");
            set_http_span_status(&root_span, response.status(), Some("invalid_request"));
            return Ok(response);
        }
    };

    let request = match messages::parse_request(&body) {
        Ok(request) => request,
        Err(err) => {
            let response = bad_request_response(err.message, "invalid_request_error");
            set_http_span_status(&root_span, response.status(), Some("invalid_request"));
            return Ok(response);
        }
    };

    let request_model_id = if request.model.trim().is_empty() || request.model.trim() == "auto" {
        None
    } else {
        Some(request.model.clone())
    };
    let provider_entry = match state.provider_registry.select_chat(
        EndpointKind::Messages,
        request_model_id.as_deref(),
        &metadata,
    ) {
        Ok(provider_entry) => provider_entry,
        Err(SelectionError::AccessDenied) => {
            let response =
                messages::error_response(StatusCode::FORBIDDEN, "access_denied", "access_denied");
            set_http_span_status(&root_span, response.status(), Some("access_denied"));
            return Ok(response);
        }
        Err(SelectionError::OutstandingBalance) => {
            let response = messages::error_response(
                StatusCode::PAYMENT_REQUIRED,
                "outstanding_balance",
                "outstanding_balance",
            );
            set_http_span_status(&root_span, response.status(), Some("outstanding_balance"));
            return Ok(response);
        }
        Err(SelectionError::ModelRequired) => {
            let message = selection_model_required_message(&format!(
                "/v1{}",
                EndpointKind::Messages.as_path()
            ));
            let response = bad_request_response(message, "invalid_request_error");
            set_http_span_status(&root_span, response.status(), Some("invalid_request"));
            return Ok(response);
        }
        Err(SelectionError::ModelNotSupported { model }) => {
            let message = selection_model_not_supported_message(&model);
            let response = bad_request_response(message, "invalid_request_error");
            set_http_span_status(&root_span, response.status(), Some("invalid_request"));
            return Ok(response);
        }
        Err(SelectionError::ModelEndpointNotSupported { model, .. }) => {
            let message = selection_model_endpoint_not_supported_message(
                &format!("/v1{}", EndpointKind::Messages.as_path()),
                &model,
            );
            let response = bad_request_response(message, "invalid_request_error");
            set_http_span_status(&root_span, response.status(), Some("invalid_request"));
            return Ok(response);
        }
    };

    let stream = request.controls.stream.unwrap_or(false);
    info!(
        "http.request.start; method=POST path=/v1/messages model_id={} stream={} trace_id={} metadata={:?}",
        request_model_id
            .as_deref()
            .unwrap_or(&provider_entry.model_id),
        stream,
        trace_id,
        metadata
    );
    if !stripped_tools.is_empty() {
        info!(
            "model api request stripped upstream-executed tools; model_id={} tools={:?} trace_id={} metadata={:?}",
            provider_entry.model_id, stripped_tools, trace_id, metadata
        );
    }

    let model_id = provider_entry.model_id.clone();
    let engine_state = EngineState {
        tool_mgr: state.tool_mgr.clone(),
        tool_invoker: state.tool_invoker.clone(),
        agent_loop_config: state.agent_loop_config.clone(),
        response_model_mode: provider_entry.response_model_mode.clone(),
    };
    let loop_result = execute::<A, M>(
        provider_entry.provider.clone(),
        model_id.clone(),
        provider_entry.label.clone(),
        request,
        &engine_state,
        metadata.clone(),
        trace_id.clone(),
    )
    .await;

    match loop_result {
        Ok(AgentLoopResult::NonStream(response)) => {
            root_span.record(
                "finish_reason",
                tracing::field::display(finish_reason_to_str(&response)),
            );
            if let Some(usage) = &response.usage {
                record_usage_attributes(&root_span, "usage", usage);
            }
            info!(
                "http.request.end; status_code=200 model_id={} trace_id={} metadata={:?}",
                model_id, trace_id, metadata
            );
            let payload = messages::serialize_non_stream(&response);
            let response = Response::builder()
                .status(StatusCode::OK)
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(payload.to_string()))
                .expect("build response");
            set_http_span_status(&root_span, response.status(), None);
            Ok(response)
        }
        Ok(AgentLoopResult::Stream { events }) => {
            let mut sse_state = messages::AnthropicSseState::new();
            let mut events = events;
            let Some(first_item) = events.next().await else {
                error!(
                    "http.request.end; status_code=500 model_id={} error=stream ended before first event trace_id={} metadata={:?}",
                    model_id, trace_id, metadata
                );
                let response = messages::error_response(
                    StatusCode::INTERNAL_SERVER_ERROR,
                    "api_error",
                    INTERNAL_SERVER_ERROR_MESSAGE,
                );
                set_http_span_status(&root_span, response.status(), Some("internal_server_error"));
                return Ok(response);
            };
            let mut sse = String::new();
            match first_item {
                Ok(event) => {
                    sse.extend(messages::AnthropicSseState::on_event(
                        &mut sse_state,
                        &event,
                    ));
                }
                Err(err) => {
                    let response = render_messages_provider_error(
                        &state, &model_id, &metadata, &trace_id, err, &root_span,
                    );
                    return Ok(response);
                }
            }
            let body_stream: Pin<Box<dyn Stream<Item = Result<Bytes, std::io::Error>> + Send>> =
                Box::pin(async_stream::try_stream! {
                    while let Some(item) = events.next().await {
                        match item {
                            Ok(event) => {
                                for frame in messages::AnthropicSseState::on_event(&mut sse_state, &event) {
                                    yield Bytes::from(frame);
                                }
                            }
                            Err(err) => {
                                for frame in render_messages_provider_error_frames(&err) {
                                    yield Bytes::from(frame);
                                }
                                return;
                            }
                        }
                    }
                    for frame in sse_state.finish() {
                        yield Bytes::from(frame);
                    }
                });
            let response = Response::builder()
                .status(StatusCode::OK)
                .header(header::CONTENT_TYPE, "text/event-stream; charset=utf-8")
                .header(header::CACHE_CONTROL, "no-cache")
                .header(header::CONNECTION, "keep-alive")
                .body(Body::from_stream(body_stream))
                .expect("build response");
            set_http_span_status(&root_span, response.status(), None);
            Ok(response)
        }
        Err(err) => {
            let response = render_messages_provider_error(
                &state, &model_id, &metadata, &trace_id, err, &root_span,
            );
            Ok(response)
        }
    }
}

/// /messages error rendering: 400 uses the anthropic envelope; everything else goes to the error policy (status wins).
fn render_messages_provider_error<A, M>(
    state: &LlmHandlerState<A, M>,
    model_id: &str,
    metadata: &M,
    trace_id: &str,
    err: ProviderError,
    root_span: &Span,
) -> Response
where
    A: Send + Sync + 'static,
    M: fmt::Debug + Clone + Serialize + Send + Sync + 'static,
{
    use crate::api::messages;
    let upstream_status = provider_error_status(&err);
    let error_message = err.to_string();
    let response = match &err {
        ProviderError::Public { status, error } if *status == StatusCode::BAD_REQUEST => {
            messages::error_response(*status, &error.r#type, &error.message)
        }
        _ => state.error_response_policy.handle_provider_error(
            EndpointKind::Messages,
            model_id,
            metadata,
            err,
        ),
    };
    let final_status = response.status();
    error!(
        "http.request.end; status_code={} upstream_status_code={} model_id={} error={} trace_id={} metadata={:?}",
        final_status.as_u16(),
        upstream_status.as_u16(),
        model_id,
        error_message,
        trace_id,
        metadata
    );
    set_http_span_status(root_span, response.status(), Some("provider_error"));
    response
}

/// Mid-stream error rendering: same as above but emits an SSE error frame.
fn render_messages_provider_error_frames(err: &ProviderError) -> Vec<String> {
    use crate::api::messages;
    let (error_type, message) = match &err {
        ProviderError::Public { error, .. } => (error.r#type.clone(), error.message.clone()),
        _ => ("api_error".to_string(), err.to_string()),
    };
    vec![messages::AnthropicSseState::error_frame(
        &error_type,
        &message,
    )]
}

/// `/v1/responses` API (OpenAI Responses): parse -> select -> execute -> serialize.
pub async fn handle_responses<A, M>(
    State(state): State<LlmHandlerState<A, M>>,
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
                    .start_request_span("POST", "/v1/responses", None);
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
            .start_request_span("POST", "/v1/responses", Some(&metadata));
    root_span.record("http.request.body.size", body.len() as i64);

    let root_span_for_error = root_span.clone();
    let instrument_span = root_span.clone();
    let response =
        handle_responses_inner::<A, M>(state, metadata.to_owned(), trace_id, body, root_span)
            .instrument(instrument_span)
            .await;
    match response {
        Ok(response) => response,
        Err(err) => {
            error!("responses completion failed: {err} {:?}", metadata);
            let message = err.to_string();
            set_http_span_status(
                &root_span_for_error,
                StatusCode::INTERNAL_SERVER_ERROR,
                Some(&message),
            );
            openai_error_response(
                StatusCode::INTERNAL_SERVER_ERROR,
                INTERNAL_SERVER_ERROR_MESSAGE,
                None,
            )
        }
    }
}

async fn handle_responses_inner<A, M>(
    state: LlmHandlerState<A, M>,
    metadata: M,
    trace_id: String,
    body: Bytes,
    root_span: Span,
) -> Result<Response, anyhow::Error>
where
    A: Send + Sync + 'static,
    M: fmt::Debug + Clone + Serialize + Send + Sync + 'static,
{
    use crate::api::responses;

    let bad_request_response = |message: &str| {
        log_bad_request(EndpointKind::Responses, message, &trace_id, &metadata);
        openai_error_response(
            StatusCode::BAD_REQUEST,
            message,
            Some("invalid_request_error"),
        )
    };

    // Strip upstream-executed tools before parsing, so hosted tool types never
    // reach the unified request; malformed bodies are rejected instead of forwarded.
    let (body, stripped_tools) = match strip_hosted_tools(EndpointKind::Responses, body) {
        Ok((body, removed)) => (body, removed),
        Err(message) => {
            let response = bad_request_response(&message);
            set_http_span_status(&root_span, response.status(), Some("invalid_request"));
            return Ok(response);
        }
    };

    let request = match responses::parse_request(&body) {
        Ok(request) => request,
        Err(err) => {
            let response = bad_request_response(&err.message);
            set_http_span_status(&root_span, response.status(), Some("invalid_request"));
            return Ok(response);
        }
    };

    let request_model_id = if request.model.trim().is_empty() || request.model.trim() == "auto" {
        None
    } else {
        Some(request.model.clone())
    };
    let provider_entry = match state.provider_registry.select_chat(
        EndpointKind::Responses,
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
                EndpointKind::Responses.as_path()
            ));
            let response = bad_request_response(&message);
            set_http_span_status(&root_span, response.status(), Some(&message));
            return Ok(response);
        }
        Err(SelectionError::ModelNotSupported { model }) => {
            let message = selection_model_not_supported_message(&model);
            let response = bad_request_response(&message);
            set_http_span_status(&root_span, response.status(), Some(&message));
            return Ok(response);
        }
        Err(SelectionError::ModelEndpointNotSupported { model, .. }) => {
            let message = selection_model_endpoint_not_supported_message(
                &format!("/v1{}", EndpointKind::Responses.as_path()),
                &model,
            );
            let response = bad_request_response(&message);
            set_http_span_status(&root_span, response.status(), Some(&message));
            return Ok(response);
        }
    };

    let stream = request.controls.stream.unwrap_or(false);
    info!(
        "http.request.start; method=POST path=/v1/responses model_id={} stream={} trace_id={} metadata={:?}",
        request_model_id
            .as_deref()
            .unwrap_or(&provider_entry.model_id),
        stream,
        trace_id,
        metadata
    );
    if !stripped_tools.is_empty() {
        info!(
            "model api request stripped upstream-executed tools; model_id={} tools={:?} trace_id={} metadata={:?}",
            provider_entry.model_id, stripped_tools, trace_id, metadata
        );
    }

    let model_id = provider_entry.model_id.clone();
    let engine_state = EngineState {
        tool_mgr: state.tool_mgr.clone(),
        tool_invoker: state.tool_invoker.clone(),
        agent_loop_config: state.agent_loop_config.clone(),
        response_model_mode: provider_entry.response_model_mode.clone(),
    };
    let loop_result = execute::<A, M>(
        provider_entry.provider.clone(),
        model_id.clone(),
        provider_entry.label.clone(),
        request,
        &engine_state,
        metadata.clone(),
        trace_id.clone(),
    )
    .await;

    match loop_result {
        Ok(AgentLoopResult::NonStream(response)) => {
            root_span.record(
                "finish_reason",
                tracing::field::display(finish_reason_to_str(&response)),
            );
            if let Some(usage) = &response.usage {
                record_usage_attributes(&root_span, "usage", usage);
            }
            info!(
                "http.request.end; status_code=200 model_id={} trace_id={} metadata={:?}",
                model_id, trace_id, metadata
            );
            let payload = responses::serialize_non_stream(&response);
            let response = Response::builder()
                .status(StatusCode::OK)
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(payload.to_string()))
                .expect("build response");
            set_http_span_status(&root_span, response.status(), None);
            Ok(response)
        }
        Ok(AgentLoopResult::Stream { events }) => {
            let mut sse_state = responses::ResponsesSseState::new();
            let mut events = events;
            let Some(first_item) = events.next().await else {
                error!(
                    "http.request.end; status_code=500 model_id={} error=stream ended before first event trace_id={} metadata={:?}",
                    model_id, trace_id, metadata
                );
                let response = openai_error_response(
                    StatusCode::INTERNAL_SERVER_ERROR,
                    INTERNAL_SERVER_ERROR_MESSAGE,
                    None,
                );
                set_http_span_status(&root_span, response.status(), Some("internal_server_error"));
                return Ok(response);
            };
            let mut sse = String::new();
            match first_item {
                Ok(event) => {
                    sse.extend(responses::ResponsesSseState::on_event(
                        &mut sse_state,
                        &event,
                    ));
                }
                Err(err) => {
                    let upstream_status = provider_error_status(&err);
                    let error_message = err.to_string();
                    let response = state.error_response_policy.handle_provider_error(
                        EndpointKind::Responses,
                        &model_id,
                        &metadata,
                        err,
                    );
                    let final_status = response.status();
                    error!(
                        "http.request.end; status_code={} upstream_status_code={} model_id={} error={} trace_id={} metadata={:?}",
                        final_status.as_u16(),
                        upstream_status.as_u16(),
                        model_id,
                        error_message,
                        trace_id,
                        metadata
                    );
                    set_http_span_status(&root_span, response.status(), Some("provider_error"));
                    return Ok(response);
                }
            }
            // Errors are delivered as response.failed SSE frames, never via the transport error channel.
            let body_stream = async_stream::stream! {
                while let Some(item) = events.next().await {
                    match item {
                        Ok(event) => {
                            for frame in responses::ResponsesSseState::on_event(&mut sse_state, &event) {
                                yield Ok::<Bytes, std::io::Error>(Bytes::from(frame));
                            }
                        }
                        Err(err) => {
                            for frame in render_responses_error_frames(&err) {
                                yield Ok(Bytes::from(frame));
                            }
                            return;
                        }
                    }
                }
                for frame in sse_state.finish() {
                    yield Ok(Bytes::from(frame));
                }
            };
            let response = Response::builder()
                .status(StatusCode::OK)
                .header(header::CONTENT_TYPE, "text/event-stream; charset=utf-8")
                .header(header::CACHE_CONTROL, "no-cache")
                .header(header::CONNECTION, "keep-alive")
                .body(Body::from_stream(body_stream))
                .expect("build response");
            set_http_span_status(&root_span, response.status(), None);
            Ok(response)
        }
        Err(err) => {
            let upstream_status = provider_error_status(&err);
            let error_message = err.to_string();
            let response = state.error_response_policy.handle_provider_error(
                EndpointKind::Responses,
                &model_id,
                &metadata,
                err,
            );
            let final_status = response.status();
            error!(
                "http.request.end; status_code={} upstream_status_code={} model_id={} error={} trace_id={} metadata={:?}",
                final_status.as_u16(),
                upstream_status.as_u16(),
                model_id,
                error_message,
                trace_id,
                metadata
            );
            set_http_span_status(&root_span, response.status(), Some("provider_error"));
            Ok(response)
        }
    }
}

fn render_responses_error_frames(err: &ProviderError) -> Vec<String> {
    use crate::api::responses;
    use crate::provider::UnifiedEvent;
    let (code, message) = match err {
        ProviderError::Public { error, .. } => (
            error.code.clone().unwrap_or_else(|| error.r#type.clone()),
            error.message.clone(),
        ),
        _ => ("server_error".to_string(), err.to_string()),
    };
    // Mid-stream errors go through the response.failed frame (same channel as normal events).
    let mut state = responses::ResponsesSseState::new();
    state.on_event(&UnifiedEvent::new(UnifiedEventKind::ResponseCreated {
        response: UnifiedResponse::new("error".into(), String::new()),
    }));
    state.on_event(&UnifiedEvent::new(UnifiedEventKind::Failed {
        error: UnifiedResponseError {
            code: Some(code),
            message,
            error_type: None,
            param: None,
            source_extensions: None,
        },
        response: None,
    }))
}

/// Non-chat endpoint state.
pub struct ModelEndpointState<A, M> {
    pub registry: crate::provider_registry::Registry,
    pub usage_handler: Arc<dyn UsageHandler<M>>,
    pub metadata_mgr: Arc<dyn MetadataMgr<A, M>>,
    pub request_span_starter: Arc<dyn RequestSpanStarter<M>>,
}

impl<A, M> Clone for ModelEndpointState<A, M> {
    fn clone(&self) -> Self {
        Self {
            registry: self.registry.clone(),
            usage_handler: self.usage_handler.clone(),
            metadata_mgr: self.metadata_mgr.clone(),
            request_span_starter: self.request_span_starter.clone(),
        }
    }
}

/// Non-chat endpoint handler: `/{*path}` all go through the passthrough dialect
/// `forward` (raw forwarding + response_model application + usage wired to on_usage).
pub async fn handle_model_endpoint<A, M>(
    Path(path): Path<String>,
    State(state): State<ModelEndpointState<A, M>>,
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
            error!("new metadata from headers: {err}");
            return model_endpoint_error_response(StatusCode::BAD_REQUEST, &err.to_string());
        }
    };
    let (root_span, trace_id) = state.request_span_starter.start_request_span(
        "POST",
        &format!("/v1/{path}"),
        Some(&metadata),
    );
    root_span.record("http.request.body.size", body.len() as i64);

    let result = handle_model_endpoint_inner::<A, M>(
        state,
        metadata,
        trace_id,
        path,
        headers,
        body,
        root_span.clone(),
    )
    .await;
    match result {
        Ok(response) => response,
        Err(err) => {
            error!("model endpoint failed: {err}");
            set_http_span_status(
                &root_span,
                StatusCode::INTERNAL_SERVER_ERROR,
                Some(&err.to_string()),
            );
            model_endpoint_error_response(StatusCode::INTERNAL_SERVER_ERROR, &err.to_string())
        }
    }
}

async fn handle_model_endpoint_inner<A, M>(
    state: ModelEndpointState<A, M>,
    metadata: M,
    trace_id: String,
    path: String,
    request_headers: HeaderMap,
    body: Bytes,
    root_span: Span,
) -> Result<Response, anyhow::Error>
where
    A: Send + Sync + 'static,
    M: fmt::Debug + Clone + Serialize + Send + Sync + 'static,
{
    // path -> endpoint; generateContent semantics are handled by the dialect.
    let endpoint = EndpointKind::from_request_path(&format!("/{path}"))
        .ok_or_else(|| anyhow!("unknown endpoint path: /{path}"))?;

    // model resolution: generateContent takes the URL segment, others take the body model field
    let model = match endpoint {
        EndpointKind::ModelsGenerateContent => {
            crate::dialects::passthrough::parse_generate_content_model(&format!("/{path}"))
                .ok_or_else(|| anyhow!("generateContent path missing model: /{path}"))?
        }
        _ => {
            let value: Value = serde_json::from_slice(&body)
                .map_err(|err| anyhow!("invalid request body: {err}"))?;
            value
                .get("model")
                .and_then(Value::as_str)
                .filter(|model| !model.trim().is_empty())
                .ok_or_else(|| anyhow!("model is required for {}", endpoint.as_path()))?
                .to_string()
        }
    };

    // Table lookup validates model existence and the endpoint's serve binding.
    let entry = state
        .registry
        .get(&model)
        .filter(|entry| entry.serves_endpoint(endpoint))
        .ok_or_else(|| anyhow!("model {model} is not enabled for {}", endpoint.as_path()))?;
    let forwarder = entry
        .forwarder
        .clone()
        .ok_or_else(|| anyhow!("model {model} is not a passthrough provider"))?;

    info!(
        "http.request.start; method=POST path=/v1/{path} model_id={model} trace_id={trace_id} metadata={:?}",
        metadata
    );

    // Strip upstream-executed tools before forwarding; the body is re-serialized
    // for the checked endpoints, so the upstream parses exactly the JSON checked here.
    let body = match strip_hosted_tools(endpoint, body) {
        Ok((body, removed)) => {
            if !removed.is_empty() {
                info!(
                    "model api request stripped upstream-executed tools; model_id={} tools={:?} trace_id={} metadata={:?}",
                    entry.model_id, removed, trace_id, metadata
                );
            }
            body
        }
        Err(message) => {
            warn!(
                "http.request.end; status_code=400 model_id={} error={} trace_id={} metadata={:?}",
                entry.model_id, message, trace_id, metadata
            );
            set_http_span_status(&root_span, StatusCode::BAD_REQUEST, Some(&message));
            return Ok(model_endpoint_error_response(
                StatusCode::BAD_REQUEST,
                &message,
            ));
        }
    };

    // usage sink: fires on_usage mid-stream for streaming responses
    let usage_handler_for_sink = state.usage_handler.clone();
    let sink_model = entry.model_id.clone();
    let sink_metadata = metadata.clone();
    let sink_trace_id = trace_id.clone();
    let usage_sink: crate::dialects::passthrough::UsageSink = Arc::new(move |usage| {
        let usage_handler = usage_handler_for_sink.clone();
        let model = sink_model.clone();
        let metadata = sink_metadata.clone();
        let trace_id = sink_trace_id.clone();
        tokio::spawn(async move {
            usage_handler
                .on_usage(
                    &model,
                    None,
                    &resolve_passthrough_request_id(None, &trace_id),
                    &trace_id,
                    metadata,
                    usage,
                )
                .await;
        });
    });

    let forwarded = forwarder
        .forward(
            endpoint,
            &format!("/{}", path),
            &request_headers,
            body,
            &ResponseModelMode::Upstream,
            &model,
            Some(usage_sink),
        )
        .await
        .map_err(|err| anyhow!("{err}"))?;

    let mut builder = Response::builder().status(forwarded.status);
    for (key, value) in forwarded.headers.iter() {
        if key != header::CONTENT_LENGTH && key != header::TRANSFER_ENCODING {
            builder = builder.header(key, value);
        }
    }

    let response = match forwarded.body {
        crate::dialects::passthrough::ForwardBody::Full(bytes) => {
            if let Some(usage) = forwarded.usage {
                state
                    .usage_handler
                    .on_usage(
                        &model,
                        None,
                        &resolve_passthrough_request_id(None, &trace_id),
                        &trace_id,
                        metadata.clone(),
                        usage,
                    )
                    .await;
            }
            let content_encoding = forwarded
                .headers
                .get(header::CONTENT_ENCODING)
                .and_then(|value| value.to_str().ok());
            if let Some(error) =
                full_response_error_for_log(forwarded.status, &bytes, content_encoding)
            {
                error!(
                    "http.request.end; status_code={} model_id={} error={} trace_id={trace_id} metadata={:?}",
                    forwarded.status.as_u16(),
                    entry.model_id,
                    error,
                    metadata
                );
            } else {
                info!(
                    "http.request.end; status_code={} model_id={} trace_id={trace_id} metadata={:?}",
                    forwarded.status.as_u16(),
                    entry.model_id,
                    metadata
                );
            }
            builder.body(Body::from(bytes)).expect("build response")
        }
        crate::dialects::passthrough::ForwardBody::Stream(stream) => {
            info!(
                "http.request.end; status_code={} model_id={} stream=true trace_id={trace_id}",
                forwarded.status.as_u16(),
                entry.model_id
            );
            builder
                .body(Body::from_stream(stream))
                .expect("build response")
        }
    };
    set_http_span_status(&root_span, response.status(), None);
    Ok(response)
}

fn full_response_error_for_log(
    status: StatusCode,
    payload: &Bytes,
    content_encoding: Option<&str>,
) -> Option<String> {
    if status.is_success() {
        return None;
    }
    Some(body_preview_for_log(payload, content_encoding))
}

fn log_bad_request<M: fmt::Debug>(
    endpoint: EndpointKind,
    message: &str,
    trace_id: &str,
    metadata: &M,
) {
    error!(
        "http.request.end; status_code=400 path=/v1{} error={} trace_id={} metadata={:?}",
        endpoint.as_path(),
        message,
        trace_id,
        metadata
    );
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
) -> Result<Cow<'a, [u8]>, IoError> {
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

/// Non-chat endpoint error rendering (JSON `{"error": {"message": ...}}`).
fn model_endpoint_error_response(status: StatusCode, message: &str) -> Response {
    let payload = serde_json::json!({"error": {"message": message}});
    Response::builder()
        .status(status)
        .header(header::CONTENT_TYPE, "application/json")
        .body(Body::from(payload.to_string()))
        .expect("build error response")
}

/// Build the non-chat endpoint router, mounted for typed configs only.
pub fn build_model_endpoint_api<A, M>(state: ModelEndpointState<A, M>) -> anyhow::Result<Router>
where
    A: Send + Sync + 'static,
    M: fmt::Debug + Clone + Serialize + Send + Sync + 'static,
{
    let app = Router::new()
        .route(
            "/{*path}",
            axum::routing::post(handle_model_endpoint::<A, M>),
        )
        .with_state(state);
    Ok(app)
}

/// Extracts the requested model from the request path and payload. Shared by the
/// non-chat endpoint handler and downstream request gates so both resolve the
/// model the same way. The path model (Gemini style) takes precedence over the
/// payload model.
pub async fn extract_request_model(
    endpoint_path: &str,
    content_type: &Option<String>,
    body: &Bytes,
) -> Option<String> {
    let path_model = parse_generate_content_model(endpoint_path);
    let body_model = parse_payload_model(content_type, body).await;
    path_model.or(body_model)
}

/// Reads the `model` field from a JSON or multipart request payload.
async fn parse_payload_model(content_type: &Option<String>, body: &Bytes) -> Option<String> {
    let content_type = content_type.as_deref()?;
    if content_type.starts_with("application/json") {
        let value: Value = serde_json::from_slice(body).ok()?;
        return value
            .get("model")
            .and_then(Value::as_str)
            .map(str::to_string);
    }
    if content_type.starts_with("multipart/form-data") {
        return parse_multipart_model(content_type, body)
            .await
            .ok()
            .flatten();
    }
    None
}

/// Reads the `model` field from a multipart payload.
async fn parse_multipart_model(content_type: &str, body: &Bytes) -> Result<Option<String>, String> {
    let boundary = parse_multipart_boundary(content_type)
        .ok_or_else(|| "multipart boundary is missing".to_string())?;
    let stream = stream::once(async move { Ok::<Bytes, MultipartError>(body.clone()) });
    let mut multipart = Multipart::new(stream, boundary);
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

/// Reads the multipart boundary from the content type.
fn parse_multipart_boundary(content_type: &str) -> Option<String> {
    content_type.split(';').find_map(|part| {
        let part = part.trim();
        part.strip_prefix("boundary=")
            .map(|value| value.trim_matches('"').to_string())
    })
}

fn finish_reason_to_str(response: &UnifiedResponse) -> &str {
    response
        .candidates
        .first()
        .and_then(|c| c.ending.as_ref())
        .map_or("stop", |ending| ending.to_chat())
}

fn provider_error_status(err: &ProviderError) -> StatusCode {
    match err {
        ProviderError::Public { status, .. } => *status,
        ProviderError::Internal {
            upstream_http_status,
            ..
        } => *upstream_http_status,
    }
}

fn response_model_for_mode(
    mode: &ResponseModelMode,
    routed_model: &str,
    upstream_model: String,
) -> String {
    match mode {
        ResponseModelMode::Upstream => {
            if upstream_model.trim().is_empty() {
                routed_model.to_string()
            } else {
                upstream_model
            }
        }
        ResponseModelMode::Fixed(model) => model.clone(),
    }
}

fn apply_response_model_to_event(
    event: &mut UnifiedEvent,
    mode: &ResponseModelMode,
    routed_model: &str,
) {
    fn apply(response: &mut UnifiedResponse, mode: &ResponseModelMode, routed_model: &str) {
        let upstream_model = std::mem::take(&mut response.model);
        response.model = response_model_for_mode(mode, routed_model, upstream_model);
    }

    match &mut event.kind {
        UnifiedEventKind::ResponseCreated { response } => {
            apply(response, mode, routed_model);
        }
        UnifiedEventKind::ResponseQueued {
            response: Some(response),
        }
        | UnifiedEventKind::ResponseInProgress {
            response: Some(response),
        }
        | UnifiedEventKind::Completed {
            response: Some(response),
        }
        | UnifiedEventKind::Incomplete {
            response: Some(response),
            ..
        }
        | UnifiedEventKind::Failed {
            response: Some(response),
            ..
        }
        | UnifiedEventKind::Cancelled {
            response: Some(response),
            ..
        } => apply(response, mode, routed_model),
        _ => {}
    }
}

fn selection_model_required_message(endpoint: &str) -> String {
    format!("model is required for {endpoint}")
}

fn selection_model_not_supported_message(model: &str) -> String {
    format!("model {model} is not supported")
}

fn selection_model_endpoint_not_supported_message(endpoint: &str, model: &str) -> String {
    format!("model {model} is not enabled for {endpoint}")
}

pub async fn build_llm_api(
    tool_mgr: Arc<dyn ToolMgr<(), ()>>,
    provider_registry: Arc<dyn crate::provider_registry::ProviderRegistry<()>>,
    tool_invoker: Arc<dyn ToolInvoker>,
    agent_loop_config: AgentLoopConfig<()>,
) -> anyhow::Result<Router> {
    build_llm_api_with_error_policy(
        tool_mgr,
        provider_registry,
        tool_invoker,
        agent_loop_config,
        Arc::new(DefaultLlmErrorResponsePolicy),
    )
    .await
}

pub async fn build_llm_api_with_error_policy(
    tool_mgr: Arc<dyn ToolMgr<(), ()>>,
    provider_registry: Arc<dyn crate::provider_registry::ProviderRegistry<()>>,
    tool_invoker: Arc<dyn ToolInvoker>,
    agent_loop_config: AgentLoopConfig<()>,
    error_response_policy: Arc<dyn LlmErrorResponsePolicy<()>>,
) -> anyhow::Result<Router> {
    let state = LlmHandlerState {
        provider_registry,
        tool_mgr,
        tool_invoker,
        metadata_mgr: Arc::new(MetadataMgrImpl::new()),
        request_span_starter: Arc::new(DefaultRequestSpanStarter),
        agent_loop_config,
        mapper_selector: Arc::new(DefaultStreamMapperSelector::default()),
        error_response_policy,
    };

    let app = axum::Router::new()
        .route(
            "/chat/completions",
            axum::routing::post(handle_chat_completions::<(), ()>),
        )
        .route("/messages", axum::routing::post(handle_messages::<(), ()>))
        .route(
            "/responses",
            axum::routing::post(handle_responses::<(), ()>),
        )
        .with_state(state);
    Ok(app)
}

#[cfg(test)]
mod tests {
    use std::cell::RefCell;
    use std::io::Write;
    use std::sync::{Arc, Once};

    use axum::body::{Body, Bytes, to_bytes};
    use axum::http::{Request, StatusCode};
    use axum::response::Response;
    use axum::routing::post;
    use axum::{Json, Router};
    use flate2::Compression;
    use flate2::write::GzEncoder;
    use log::{Level, LevelFilter, Log, Metadata, Record, set_logger, set_max_level};
    use serde_json::{Value, json};
    use tokio::net::TcpListener;
    use tower::ServiceExt;

    use super::{
        DefaultLlmErrorResponsePolicy, LlmErrorResponsePolicy, ModelEndpointState,
        apply_response_model_to_event, body_preview_for_log, build_llm_api,
        build_model_endpoint_api, decode_payload_for_log, extract_request_model,
        full_response_error_for_log, provider_error_status, response_model_for_mode,
        selection_model_endpoint_not_supported_message, selection_model_not_supported_message,
        selection_model_required_message,
    };
    use crate::agent_loop::AgentLoopConfig;
    use crate::connector::TcpConnector;
    use crate::metadata_mgr::MetadataMgrImpl;
    use crate::openai_types::ErrorDetail;
    use crate::provider::{ProviderError, UnifiedEvent, UnifiedEventKind, UnifiedResponse};
    use crate::provider_registry::{ByEndpointModel, ConfigProviderRegistry, build_registry};
    use crate::serve_config::{
        DefaultsConfig, ProviderConfig, ResponseModelMode, derive_endpoints_from_typed,
    };
    use crate::tool_invoker::ConnToolInvoker;
    use crate::tool_mgr::ToolMgrImpl;
    use crate::trace::DefaultRequestSpanStarter;
    use crate::usage_handler::NoopUsageHandler;
    use crate::utils::MAX_LOG_BODY_BYTES;

    thread_local! {
        static ERROR_LOGS: RefCell<Vec<String>> = const { RefCell::new(Vec::new()) };
    }

    // Record actual ERROR output per test thread, with DEBUG disabled.
    struct ErrorLogRecorder;

    impl Log for ErrorLogRecorder {
        fn enabled(&self, metadata: &Metadata<'_>) -> bool {
            metadata.level() == Level::Error
        }

        fn log(&self, record: &Record<'_>) {
            if self.enabled(record.metadata()) {
                ERROR_LOGS.with(|logs| logs.borrow_mut().push(record.args().to_string()));
            }
        }

        fn flush(&self) {}
    }

    fn reset_error_logs() {
        static LOGGER: ErrorLogRecorder = ErrorLogRecorder;
        static INIT: Once = Once::new();
        INIT.call_once(|| {
            set_logger(&LOGGER).expect("install error log recorder");
            set_max_level(LevelFilter::Error);
        });
        ERROR_LOGS.with(|logs| logs.borrow_mut().clear());
    }

    /// Verifies the shared extractor prefers the Gemini path model over the payload.
    #[tokio::test]
    async fn extract_request_model_prefers_path_model_over_payload() {
        let content_type = Some("application/json".to_string());
        let body = Bytes::from_static(br#"{"model":"gpt-4o"}"#);

        let model =
            extract_request_model("/models/gemini-flash:generateContent", &content_type, &body)
                .await;

        assert_eq!(model.as_deref(), Some("gemini-flash"));
    }

    /// Verifies the shared extractor falls back to JSON and multipart payloads.
    #[tokio::test]
    async fn extract_request_model_reads_json_and_multipart_payload() {
        let json_body = Bytes::from_static(br#"{"model":"gpt-4o"}"#);
        let model = extract_request_model(
            "/responses",
            &Some("application/json".to_string()),
            &json_body,
        )
        .await;
        assert_eq!(model.as_deref(), Some("gpt-4o"));

        let multipart_body = Bytes::from_static(
            b"--boundary\r\nContent-Disposition: form-data; name=\"model\"\r\n\r\nwhisper-2\r\n--boundary--\r\n",
        );
        let model = extract_request_model(
            "/audio/transcriptions",
            &Some("multipart/form-data; boundary=boundary".to_string()),
            &multipart_body,
        )
        .await;
        assert_eq!(model.as_deref(), Some("whisper-2"));
    }

    /// Verifies the shared extractor returns none for payloads without a model.
    #[tokio::test]
    async fn extract_request_model_returns_none_without_model() {
        let body = Bytes::from_static(br#"{"messages":[]}"#);

        let model =
            extract_request_model("/responses", &Some("application/json".to_string()), &body).await;

        assert_eq!(model, None);
    }

    async fn error_test_app(providers: &[ProviderConfig]) -> Router {
        let endpoints = derive_endpoints_from_typed(providers, &DefaultsConfig::default());
        let strategy = Arc::new(ByEndpointModel::new(
            endpoints
                .into_iter()
                .map(|endpoint| (endpoint.endpoint().expect("endpoint"), endpoint))
                .collect(),
        ));
        let registry = ConfigProviderRegistry::<()>::from_entries(providers, strategy)
            .expect("provider registry");
        build_llm_api(
            Arc::new(ToolMgrImpl::new()),
            Arc::new(registry),
            Arc::new(ConnToolInvoker::new(Arc::new(TcpConnector::new(
                "127.0.0.1:1",
            )))),
            AgentLoopConfig::default(),
        )
        .await
        .expect("LLM router")
    }

    fn error_test_request(path: &str, stream: bool) -> Value {
        match path {
            "/responses" => json!({"model":"error-model", "input":"hello", "stream":stream}),
            "/messages" => json!({"model":"error-model", "max_tokens":128,
                "messages":[{"role":"user", "content":"hello"}], "stream":stream}),
            _ => json!({"model":"error-model",
                "messages":[{"role":"user", "content":"hello"}], "stream":stream}),
        }
    }

    async fn assert_bad_request_logged(app: Router, path: &str, body: Bytes) -> String {
        reset_error_logs();
        let response = app
            .oneshot(
                Request::builder()
                    .method("POST")
                    .uri(path)
                    .header("content-type", "application/json")
                    .body(Body::from(body))
                    .expect("request"),
            )
            .await
            .expect("HTTP response");
        assert_eq!(response.status(), StatusCode::BAD_REQUEST, "{path}");
        let bytes = to_bytes(response.into_body(), usize::MAX)
            .await
            .expect("response body");
        let payload: Value = serde_json::from_slice(&bytes).expect("error payload");
        let message = payload["error"]["message"]
            .as_str()
            .expect("concrete error message");
        assert!(!message.is_empty());
        ERROR_LOGS.with(|logs| {
            let logs = logs.borrow();
            assert!(
                logs.iter()
                    .any(|line| { line.contains("status_code=400") && line.contains(message) }),
                "{path}: missing error details {message:?} in {logs:?}"
            );
        });
        message.to_owned()
    }

    #[tokio::test]
    async fn upstream_400_logs_details_for_chat_messages_and_responses_without_debug() {
        let listener = TcpListener::bind("127.0.0.1:0")
            .await
            .expect("bind upstream");
        let address = listener.local_addr().expect("upstream address");
        let upstream = tokio::spawn(async move {
            axum::serve(
                listener,
                Router::new().fallback(post(|| async {
                    (
                        StatusCode::BAD_REQUEST,
                        Json(json!({"error": {
                            "message":"temperature must be between 0 and 1",
                            "type":"invalid_request_error", "param":"temperature"
                        }})),
                    )
                })),
            )
            .await
            .expect("serve upstream");
        });

        for kind in ["openai-chat-completions", "openai-responses"] {
            let provider: ProviderConfig = serde_json::from_value(json!({
                "type":kind, "model_id":"error-model",
                "params":{"base_url":format!("http://{address}/v1"), "model":"error-model"}
            }))
            .expect("provider config");
            let app = error_test_app(&[provider]).await;
            for path in ["/chat/completions", "/messages", "/responses"] {
                for stream in [false, true] {
                    let body = Bytes::from(
                        serde_json::to_vec(&error_test_request(path, stream)).expect("body"),
                    );
                    let message = assert_bad_request_logged(app.clone(), path, body).await;
                    assert_eq!(message, "temperature must be between 0 and 1");
                }
            }
        }
        upstream.abort();
    }

    #[tokio::test]
    async fn local_400_logs_details_for_all_chat_endpoints() {
        let provider: ProviderConfig = serde_json::from_value(json!({
            "type":"openai-chat-completions", "model_id":"error-model",
            "params":{"base_url":"http://127.0.0.1:1/v1", "model":"error-model"}
        }))
        .expect("provider config");
        let app = error_test_app(&[provider]).await;
        for path in ["/chat/completions", "/messages", "/responses"] {
            for model in ["", "unknown-model"] {
                let mut request = error_test_request(path, false);
                request["model"] = json!(model);
                let body = Bytes::from(serde_json::to_vec(&request).expect("body"));
                assert_bad_request_logged(app.clone(), path, body).await;
            }
            assert_bad_request_logged(app.clone(), path, Bytes::from_static(b"{invalid")).await;
        }
    }

    #[tokio::test]
    async fn passthrough_400_logs_details_without_changing_response() {
        let listener = TcpListener::bind("127.0.0.1:0")
            .await
            .expect("bind upstream");
        let address = listener.local_addr().expect("upstream address");
        let upstream = tokio::spawn(async move {
            axum::serve(
                listener,
                Router::new().fallback(post(|| async {
                    (
                        StatusCode::BAD_REQUEST,
                        Json(json!({"error": {
                            "message":"input must not be empty"
                        }})),
                    )
                })),
            )
            .await
            .expect("serve upstream");
        });
        let provider: ProviderConfig = serde_json::from_value(json!({
            "type":"passthrough", "model_id":"error-model", "serve":["/embeddings"],
            "params":{"base_url":format!("http://{address}/v1"), "model":"error-model"}
        }))
        .expect("provider config");
        let app = build_model_endpoint_api(ModelEndpointState::<(), ()> {
            registry: build_registry(&[provider]).expect("registry"),
            usage_handler: Arc::new(NoopUsageHandler),
            metadata_mgr: Arc::new(MetadataMgrImpl::new()),
            request_span_starter: Arc::new(DefaultRequestSpanStarter),
        })
        .expect("passthrough router");
        let message = assert_bad_request_logged(
            app,
            "/embeddings",
            Bytes::from_static(br#"{"model":"error-model","input":""}"#),
        )
        .await;
        assert_eq!(message, "input must not be empty");
        upstream.abort();
    }

    #[tokio::test]
    async fn hosted_tools_are_stripped_before_parsing_and_forwarding() {
        let captured = Arc::new(tokio::sync::Mutex::new(Vec::<Value>::new()));
        let listener = TcpListener::bind("127.0.0.1:0")
            .await
            .expect("bind upstream");
        let address = listener.local_addr().expect("upstream address");
        let capture_for_upstream = captured.clone();
        let upstream = tokio::spawn(async move {
            axum::serve(
                listener,
                Router::new().fallback(post(move |Json(body): Json<Value>| {
                    let captured = capture_for_upstream.clone();
                    async move {
                        captured.lock().await.push(body);
                        Json(json!({
                            "id":"chatcmpl-1", "object":"chat.completion", "created":1767225600,
                            "model":"error-model",
                            "choices":[{"index":0, "finish_reason":"stop",
                                "message":{"role":"assistant", "content":"ok"}}],
                            "usage":{"prompt_tokens":1, "completion_tokens":1, "total_tokens":2}
                        }))
                    }
                })),
            )
            .await
            .expect("serve upstream");
        });

        let provider: ProviderConfig = serde_json::from_value(json!({
            "type":"openai-chat-completions", "model_id":"error-model",
            "params":{"base_url":format!("http://{address}/v1"), "model":"error-model"}
        }))
        .expect("provider config");
        let app = error_test_app(&[provider]).await;

        // /messages: mcp_servers/container and hosted tool types never reach the upstream.
        let messages_body = json!({
            "model":"error-model", "max_tokens":128,
            "messages":[{"role":"user", "content":"hello"}],
            "mcp_servers":[{"type":"url", "url":"https://example.com/mcp", "name":"docs"}],
            "container":{"id":"c1"},
            "tools":[
                {"name":"lookup", "input_schema":{"type":"object"}},
                {"type":"web_search_20250305", "name":"web_search", "max_uses":5}
            ]
        });
        let response = app
            .clone()
            .oneshot(
                Request::builder()
                    .method("POST")
                    .uri("/messages")
                    .header("content-type", "application/json")
                    .body(Body::from(messages_body.to_string()))
                    .expect("request"),
            )
            .await
            .expect("response");
        assert_eq!(response.status(), StatusCode::OK);

        // /responses: hosted tools are stripped; client function tools are kept.
        let responses_body = json!({
            "model":"error-model", "input":"hello",
            "tool_choice":"auto",
            "tools":[
                {"type":"function", "name":"lookup", "parameters":{"type":"object"}},
                {"type":"web_search"},
                {"type":"code_interpreter", "container":{"type":"auto"}}
            ]
        });
        let response = app
            .clone()
            .oneshot(
                Request::builder()
                    .method("POST")
                    .uri("/responses")
                    .header("content-type", "application/json")
                    .body(Body::from(responses_body.to_string()))
                    .expect("request"),
            )
            .await
            .expect("response");
        assert_eq!(response.status(), StatusCode::OK);

        // Malformed tools are rejected by the strip layer instead of being forwarded.
        for path in ["/messages", "/responses"] {
            let malformed = json!({"model":"error-model", "tools":{"type":"web_search"}});
            let body = Bytes::from(serde_json::to_vec(&malformed).expect("body"));
            assert_bad_request_logged(app.clone(), path, body).await;
        }

        let bodies = captured.lock().await.clone();
        assert_eq!(bodies.len(), 2, "two forwarded requests");
        let forwarded_messages = &bodies[0];
        assert!(forwarded_messages.get("mcp_servers").is_none());
        assert!(forwarded_messages.get("container").is_none());
        let tools = forwarded_messages["tools"].as_array().expect("tools array");
        assert!(
            tools
                .iter()
                .all(|tool| !tool.to_string().contains("web_search")),
            "hosted web_search tool must be stripped: {tools:?}"
        );
        let forwarded_responses = &bodies[1];
        let tools = forwarded_responses["tools"]
            .as_array()
            .expect("tools array");
        assert_eq!(
            tools.as_slice(),
            &[json!({"type":"function",
                "function":{"name":"lookup", "parameters":{"type":"object"}}})]
        );

        upstream.abort();
    }

    #[test]
    fn full_response_error_for_log_includes_json_error_body() {
        let payload = Bytes::from_static(br#"{"error":{"message":"invalid request"}}"#);
        for status in [
            StatusCode::BAD_REQUEST,
            StatusCode::TOO_MANY_REQUESTS,
            StatusCode::INTERNAL_SERVER_ERROR,
        ] {
            assert_eq!(
                full_response_error_for_log(status, &payload, None).as_deref(),
                Some(r#"{"error":{"message":"invalid request"}}"#)
            );
        }
        assert_eq!(
            full_response_error_for_log(StatusCode::OK, &payload, None),
            None
        );
    }

    #[test]
    fn full_response_error_for_log_decodes_compressed_error_body() {
        let raw = br#"{"error":{"message":"invalid request"}}"#;
        let mut encoder = GzEncoder::new(Vec::new(), Compression::default());
        encoder.write_all(raw).expect("write gzip error body");
        let payload = Bytes::from(encoder.finish().expect("finish gzip error body"));

        let error = full_response_error_for_log(StatusCode::BAD_REQUEST, &payload, Some("gzip"));
        assert_eq!(
            error.as_deref(),
            Some(r#"{"error":{"message":"invalid request"}}"#)
        );
        let decoded = decode_payload_for_log(&payload, Some("gzip")).expect("decode original body");
        assert_eq!(decoded.as_ref(), raw);
    }

    #[test]
    fn body_preview_for_log_handles_binary_and_truncated_utf8() {
        let binary = Bytes::from_static(&[0xff, 0xfe, 0x00]);
        assert_eq!(
            body_preview_for_log(&binary, None),
            "<non-utf8 body: len=3 first_bytes_hex=fffe00>"
        );
        let payload = Bytes::from("中".repeat(MAX_LOG_BODY_BYTES));
        let preview = body_preview_for_log(&payload, None);
        assert!(preview.contains("...[truncated "));
        assert!(preview.starts_with("中"));
        assert!(preview.ends_with("中"));
        assert!(preview.len() < MAX_LOG_BODY_BYTES + 100);
    }

    #[test]
    fn body_preview_for_log_reports_failed_compression_decoding() {
        let payload = Bytes::from_static(&[0xff, 0xfe, 0x00]);
        assert_eq!(
            body_preview_for_log(&payload, Some("gzip")),
            "<non-utf8 body: len=3 first_bytes_hex=fffe00, content_encoding=gzip, decode_failed=true>"
        );
    }

    #[test]
    fn decode_payload_for_log_bounds_decompressed_body() {
        let raw = vec![b'a'; MAX_LOG_BODY_BYTES * 2];
        let mut encoder = GzEncoder::new(Vec::new(), Compression::default());
        encoder.write_all(&raw).expect("write large body");
        let payload = Bytes::from(encoder.finish().expect("finish gzip body"));
        let decoded = decode_payload_for_log(&payload, Some("gzip")).expect("decode bounded body");
        assert_eq!(decoded.as_ref(), &raw[..MAX_LOG_BODY_BYTES]);
    }

    struct AlwaysTeapotPolicy;

    impl<M> LlmErrorResponsePolicy<M> for AlwaysTeapotPolicy {
        fn handle_provider_error(
            &self,
            _endpoint: crate::serve_config::EndpointKind,
            _model_id: &str,
            _metadata: &M,
            _err: ProviderError,
        ) -> Response {
            Response::builder()
                .status(StatusCode::IM_A_TEAPOT)
                .body(axum::body::Body::from("teapot"))
                .expect("build teapot response")
        }
    }

    #[test]
    fn provider_error_status_keeps_public_upstream_status() {
        let err = ProviderError::Public {
            status: StatusCode::TOO_MANY_REQUESTS,
            error: ErrorDetail {
                message: "rate limited".to_string(),
                r#type: "rate_limit_error".to_string(),
                code: Some("rate_limit".to_string()),
                param: None,
            },
        };

        assert_eq!(provider_error_status(&err), StatusCode::TOO_MANY_REQUESTS);
    }

    #[test]
    fn provider_error_status_keeps_internal_upstream_status() {
        let err = ProviderError::internal_with_upstream_status(
            StatusCode::SERVICE_UNAVAILABLE,
            "upstream timeout",
        );

        assert_eq!(provider_error_status(&err), StatusCode::SERVICE_UNAVAILABLE);
    }

    #[test]
    fn default_llm_error_response_policy_passthroughs_bad_request() {
        let policy = DefaultLlmErrorResponsePolicy;
        let response = policy.handle_provider_error(
            crate::serve_config::EndpointKind::ChatCompletions,
            "gpt-test",
            &(),
            ProviderError::Public {
                status: StatusCode::BAD_REQUEST,
                error: ErrorDetail {
                    message: "invalid_request".to_string(),
                    r#type: "invalid_request_error".to_string(),
                    code: None,
                    param: None,
                },
            },
        );

        assert_eq!(response.status(), StatusCode::BAD_REQUEST);
    }

    #[test]
    fn default_llm_error_response_policy_normalizes_rate_limit_to_internal_server_error() {
        let policy = DefaultLlmErrorResponsePolicy;
        let response = policy.handle_provider_error(
            crate::serve_config::EndpointKind::ChatCompletions,
            "gpt-test",
            &(),
            ProviderError::Public {
                status: StatusCode::TOO_MANY_REQUESTS,
                error: ErrorDetail {
                    message: "rate limited".to_string(),
                    r#type: "rate_limit_error".to_string(),
                    code: Some("rate_limit".to_string()),
                    param: None,
                },
            },
        );

        assert_eq!(response.status(), StatusCode::INTERNAL_SERVER_ERROR);
    }

    #[test]
    fn custom_llm_error_response_policy_can_override_status() {
        let policy = AlwaysTeapotPolicy;
        let response = policy.handle_provider_error(
            crate::serve_config::EndpointKind::ChatCompletions,
            "gpt-test",
            &(),
            ProviderError::internal("boom"),
        );

        assert_eq!(response.status(), StatusCode::IM_A_TEAPOT);
    }

    #[test]
    fn selection_model_required_message_includes_endpoint() {
        assert_eq!(
            selection_model_required_message("/v1/chat/completions"),
            "model is required for /v1/chat/completions"
        );
    }

    #[test]
    fn selection_error_messages_distinguish_model_and_endpoint_support() {
        assert_eq!(
            selection_model_not_supported_message("gpt-x"),
            "model gpt-x is not supported"
        );
        assert_eq!(
            selection_model_endpoint_not_supported_message("/v1/chat/completions", "gpt-x"),
            "model gpt-x is not enabled for /v1/chat/completions"
        );
    }

    #[test]
    fn response_model_for_mode_prefers_upstream_when_present() {
        let model = response_model_for_mode(
            &ResponseModelMode::Upstream,
            "routed-model",
            "upstream-model".to_string(),
        );

        assert_eq!(model, "upstream-model");
    }

    #[test]
    fn response_model_for_mode_supports_explicit_model() {
        let fixed = response_model_for_mode(
            &ResponseModelMode::Fixed("fixed-model".to_string()),
            "routed-model",
            "upstream-model".to_string(),
        );

        assert_eq!(fixed, "fixed-model");
    }

    #[test]
    fn response_model_override_applies_to_stream_events() {
        let mut event = UnifiedEvent::new(UnifiedEventKind::ResponseCreated {
            response: UnifiedResponse::new("resp-1".to_string(), "upstream-model".to_string()),
        });

        apply_response_model_to_event(
            &mut event,
            &ResponseModelMode::Fixed("fixed-model".to_string()),
            "routed-model",
        );

        let UnifiedEventKind::ResponseCreated { response } = event.kind else {
            panic!("expected response created event");
        };
        assert_eq!(response.model, "fixed-model");
    }

    fn apply_max_tokens(body: serde_json::Value) -> Option<i32> {
        let bytes = body.to_string();
        let request =
            crate::api::chat_completions::parse_request(bytes.as_bytes()).expect("valid request");
        request.options.max_output_tokens.map(|value| value as i32)
    }

    /// Client legacy `max_tokens` reaches the IR as `max_output_tokens` (claude-* upstreams
    /// map it back to native `max_tokens`, others receive `max_completion_tokens`).
    #[test]
    fn legacy_max_tokens_becomes_max_output_tokens() {
        let body = serde_json::json!({
            "model": "claude-haiku-5-5",
            "messages": [],
            "max_tokens": 32000,
        });

        assert_eq!(apply_max_tokens(body), Some(32000));
    }

    #[test]
    fn max_completion_tokens_wins_over_max_tokens() {
        let body = serde_json::json!({
            "model": "claude-haiku-5-5",
            "messages": [],
            "max_tokens": 32000,
            "max_completion_tokens": 1000,
        });

        assert_eq!(apply_max_tokens(body), Some(1000));
    }

    #[test]
    fn non_claude_max_tokens_is_also_mapped() {
        let body = serde_json::json!({
            "model": "glm-5.3-flash",
            "messages": [],
            "max_tokens": 32000,
        });

        assert_eq!(apply_max_tokens(body), Some(32000));
    }
}

#[cfg(test)]
mod execute_tests {
    use std::collections::HashMap;
    use std::sync::Arc;

    use async_trait::async_trait;
    use axum::http::StatusCode;
    use futures_core::Stream;
    use futures_util::StreamExt;
    use std::pin::Pin;

    use super::{EngineState, execute};
    use crate::agent_loop::AgentLoopConfig;
    use crate::agent_loop::AgentLoopResult;
    use crate::provider::{
        Provider, ProviderError, UnifiedEvent, UnifiedEventKind, UnifiedRequest, UnifiedResponse,
    };
    use crate::serve_config::ResponseModelMode;
    use crate::tool_invoker::ToolInvoker;
    use crate::tool_mgr::ToolMgr;

    struct StaticProvider {
        response: UnifiedResponse,
    }

    #[async_trait]
    impl Provider<()> for StaticProvider {
        fn model_id(&self) -> &str {
            "upstream-model"
        }

        async fn complete(
            &self,
            _request: UnifiedRequest,
            _metadata: &(),
        ) -> Result<UnifiedResponse, ProviderError> {
            Ok(self.response.clone())
        }

        async fn stream<'a>(
            &'a self,
            _request: UnifiedRequest,
            _metadata: &(),
        ) -> Result<
            Pin<Box<dyn Stream<Item = Result<UnifiedEvent, ProviderError>> + Send + 'a>>,
            ProviderError,
        > {
            Err(ProviderError::internal("stream unused in test"))
        }
    }

    struct EmptyToolMgr;

    #[async_trait]
    impl ToolMgr<(), ()> for EmptyToolMgr {
        async fn upsert_tool(
            &self,
            _tool_name: String,
            _schema: String,
            _auth_info: &(),
        ) -> anyhow::Result<()> {
            Ok(())
        }

        async fn list_tools(&self, _metadata: &()) -> anyhow::Result<HashMap<String, String>> {
            Ok(HashMap::new())
        }
    }

    fn engine_state(response_model_mode: ResponseModelMode) -> EngineState<(), ()> {
        EngineState {
            tool_mgr: Arc::new(EmptyToolMgr),
            tool_invoker: Arc::new(NoopToolInvoker),
            agent_loop_config: AgentLoopConfig::default(),
            response_model_mode,
        }
    }

    fn unified_response() -> UnifiedResponse {
        crate::api::response_from_value(&serde_json::json!({"id":"req-1","model":"upstream-model","created":1767225600,"choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}),crate::provider::UnifiedSourceProtocol::ChatCompletions).expect("response")
    }

    fn chat_request() -> UnifiedRequest {
        crate::api::request_from_value(&serde_json::json!({"model":"client-model","messages":[{"role":"user","content":"hi"}]}),crate::provider::UnifiedSourceProtocol::ChatCompletions).expect("request")
    }

    #[tokio::test]
    async fn execute_applies_response_model_at_ir_exit() {
        let provider = Arc::new(StaticProvider {
            response: unified_response(),
        });
        // fixed mode: the exit model is rewritten to the configured model
        let result = execute(
            provider.clone(),
            "routed-model".to_string(),
            None,
            chat_request(),
            &engine_state(ResponseModelMode::Fixed("fixed-model".to_string())),
            (),
            "trace-1".to_string(),
        )
        .await
        .expect("execute");
        let AgentLoopResult::NonStream(response) = result else {
            panic!("expected non-stream");
        };
        assert_eq!(response.model, "fixed-model");

        // upstream mode: keep the upstream model
        let result = execute(
            provider,
            "routed-model".to_string(),
            None,
            chat_request(),
            &engine_state(ResponseModelMode::Upstream),
            (),
            "trace-1".to_string(),
        )
        .await
        .expect("execute");
        let AgentLoopResult::NonStream(response) = result else {
            panic!("expected non-stream");
        };
        assert_eq!(response.model, "upstream-model");
    }

    #[tokio::test]
    async fn execute_rejects_invalid_request_as_public_400() {
        let provider = Arc::new(StaticProvider {
            response: unified_response(),
        });
        let mut request = chat_request();
        request.input.clear();

        let err = execute(
            provider,
            "m".to_string(),
            None,
            request,
            &engine_state(ResponseModelMode::Upstream),
            (),
            "trace-1".to_string(),
        )
        .await
        .err()
        .expect("empty messages must be rejected");

        let ProviderError::Public { status, error } = err else {
            panic!("expected public error");
        };
        assert_eq!(status, StatusCode::BAD_REQUEST);
        assert_eq!(error.r#type, "invalid_request_error");
    }

    #[tokio::test]
    async fn execute_forces_stream_include_usage() {
        let mut request = chat_request();
        request.controls.stream = Some(true);

        // StaticProvider.complete returns directly, but execute rewrites the request first;
        // a capturing provider asserts that stream_options is filled in.
        struct CaptureProvider(Arc<tokio::sync::Mutex<Option<UnifiedRequest>>>);

        #[async_trait]
        impl Provider<()> for CaptureProvider {
            fn model_id(&self) -> &str {
                "upstream-model"
            }

            async fn complete(
                &self,
                request: UnifiedRequest,
                _metadata: &(),
            ) -> Result<UnifiedResponse, ProviderError> {
                *self.0.lock().await = Some(request);
                Ok(unified_response())
            }

            async fn stream<'a>(
                &'a self,
                request: UnifiedRequest,
                _metadata: &(),
            ) -> Result<
                Pin<Box<dyn Stream<Item = Result<UnifiedEvent, ProviderError>> + Send + 'a>>,
                ProviderError,
            > {
                *self.0.lock().await = Some(request);
                Ok(Box::pin(futures_util::stream::once(async {
                    Ok(UnifiedEvent::new(UnifiedEventKind::Completed {
                        response: None,
                    }))
                })))
            }
        }

        let captured = Arc::new(tokio::sync::Mutex::new(None));
        let provider = Arc::new(CaptureProvider(captured.clone()));
        let mut request = chat_request();
        request.controls.stream = Some(true);

        let result = execute(
            provider,
            "m".to_string(),
            None,
            request,
            &engine_state(ResponseModelMode::Upstream),
            (),
            "trace-1".to_string(),
        )
        .await
        .expect("execute");
        let AgentLoopResult::Stream { mut events } = result else {
            panic!("expected stream");
        };
        // run_agent_loop_stream is lazy: poll once to trigger the provider.stream call.
        let _ = events.next().await;

        let captured_request = captured.lock().await.clone().expect("captured");
        assert_eq!(
            captured_request
                .controls
                .stream_options
                .expect("stream options")["include_usage"],
            serde_json::Value::Bool(true)
        );
    }

    struct NoopToolInvoker;

    #[async_trait]
    impl ToolInvoker for NoopToolInvoker {
        async fn invoke(
            &self,
            _headers: crate::types::RequestHeaders,
            _request: crate::types::ToolRequest,
        ) -> crate::types::ToolResponse {
            crate::types::ToolResponse {
                result: serde_json::Value::Null,
                error_msg: None,
            }
        }
    }
}

#[cfg(test)]
mod messages_golden_tests {
    use std::sync::Arc;

    use crate::tool_invoker::ToolInvoker;
    use crate::tool_mgr::ToolMgr;

    use async_trait::async_trait;
    use axum::body::Body;
    use axum::http::{Request, StatusCode};
    use tower::ServiceExt;

    use crate::agent_loop::AgentLoopConfig;
    use crate::llm_api::{DefaultLlmErrorResponsePolicy, build_llm_api_with_error_policy};
    use crate::provider_registry::{ByEndpointModel, ConfigProviderRegistry};
    use crate::serve_config::{DefaultsConfig, ProviderConfig, derive_endpoints_from_typed};

    fn typed_entry(json: &str) -> ProviderConfig {
        serde_json::from_str(json).expect("typed entry")
    }

    async fn messages_app() -> axum::Router {
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0")
            .await
            .expect("bind upstream");
        let address = listener.local_addr().expect("upstream address");
        tokio::spawn(async move {
            let upstream = axum::Router::new().route("/v1/chat/completions", axum::routing::post(|| async {
                axum::Json(serde_json::json!({
                    "id": "msg_golden", "object": "chat.completion", "created": 1767225600,
                    "model": "claude-golden", "choices": [{"index": 0, "finish_reason": "tool_calls",
                    "message": {"role": "assistant", "content": "", "tool_calls": [{"id": "toolu_golden",
                    "type": "function", "function": {"name": "shell", "arguments": "{\"cmd\":\"ls\"}"}}]}}],
                    "usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
                }))
            }));
            axum::serve(listener, upstream)
                .await
                .expect("serve upstream");
        });
        let providers = vec![typed_entry(&format!(
            r#"{{"type": "openai-chat-completions", "model_id": "claude-golden",
                "params": {{"base_url": "http://{address}/v1",
                "model": "claude-golden"}}}}"#
        ))];
        let endpoints = derive_endpoints_from_typed(
            &providers,
            &DefaultsConfig {
                model: Some("claude-golden".to_string()),
            },
        );
        let strategy = Arc::new(ByEndpointModel::new(
            endpoints
                .into_iter()
                .map(|endpoint| (endpoint.endpoint().expect("endpoint"), endpoint))
                .collect(),
        ));
        let registry =
            ConfigProviderRegistry::<()>::from_entries(&providers, strategy).expect("registry");
        build_llm_api_with_error_policy(
            Arc::new(EmptyToolMgr),
            Arc::new(registry),
            Arc::new(NoopInvoker),
            AgentLoopConfig::default(),
            Arc::new(DefaultLlmErrorResponsePolicy),
        )
        .await
        .expect("app")
    }

    struct EmptyToolMgr;

    #[async_trait]
    impl ToolMgr<(), ()> for EmptyToolMgr {
        async fn upsert_tool(
            &self,
            _tool_name: String,
            _schema: String,
            _auth_info: &(),
        ) -> anyhow::Result<()> {
            Ok(())
        }

        async fn list_tools(
            &self,
            _metadata: &(),
        ) -> anyhow::Result<std::collections::HashMap<String, String>> {
            Ok(Default::default())
        }
    }

    struct NoopInvoker;

    #[async_trait]
    impl ToolInvoker for NoopInvoker {
        async fn invoke(
            &self,
            _headers: crate::types::RequestHeaders,
            _request: crate::types::ToolRequest,
        ) -> crate::types::ToolResponse {
            crate::types::ToolResponse {
                result: serde_json::Value::Null,
                error_msg: None,
            }
        }
    }

    /// Client tool loop golden: tool_result goes in and tool_use comes out on /messages.
    #[tokio::test]
    async fn messages_tool_loop_golden() {
        let app = messages_app().await;

        // First hop: the model returns tool_use (non-streaming).
        let request_body = serde_json::json!({
            "model": "claude-golden",
            "max_tokens": 100,
            "messages": [{"role": "user", "content": "run shell"}]
        });
        let response = app
            .clone()
            .oneshot(
                Request::builder()
                    .method("POST")
                    .uri("/messages")
                    .header("content-type", "application/json")
                    .body(Body::from(request_body.to_string()))
                    .expect("request"),
            )
            .await
            .expect("response");
        assert_eq!(response.status(), StatusCode::OK);
        let bytes = axum::body::to_bytes(response.into_body(), usize::MAX)
            .await
            .expect("body");
        let value: serde_json::Value = serde_json::from_slice(&bytes).expect("json");
        assert_eq!(value["type"], "message");
        assert_eq!(value["stop_reason"], "tool_use");
        let content = value["content"].as_array().expect("content array");
        assert!(
            content.iter().any(|block| {
                block["type"] == "tool_use"
                    && block["id"] == "toolu_golden"
                    && block["name"] == "shell"
                    && block["input"] == serde_json::json!({"cmd": "ls"})
            }),
            "tool_use block missing: {value}"
        );

        // Second hop: the client returns tool_result -> IR tool message (the engine can keep looping).
        let follow_up = serde_json::json!({
            "model": "claude-golden",
            "max_tokens": 100,
            "messages": [
                {"role": "user", "content": "run shell"},
                {"role": "assistant", "content": [
                    {"type": "tool_use", "id": "toolu_golden", "name": "shell", "input": {"cmd": "ls"}}
                ]},
                {"role": "user", "content": [
                    {"type": "tool_result", "tool_use_id": "toolu_golden", "content": "ok"}
                ]}
            ]
        });
        let response = app
            .oneshot(
                Request::builder()
                    .method("POST")
                    .uri("/messages")
                    .header("content-type", "application/json")
                    .body(Body::from(follow_up.to_string()))
                    .expect("request"),
            )
            .await
            .expect("response");
        assert_eq!(response.status(), StatusCode::OK);
        let bytes = axum::body::to_bytes(response.into_body(), usize::MAX)
            .await
            .expect("body");
        let value: serde_json::Value = serde_json::from_slice(&bytes).expect("json");
        assert_eq!(value["stop_reason"], "tool_use");
    }
}
