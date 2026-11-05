use std::fmt;
use std::pin::Pin;
use std::sync::Arc;

use anyhow::Context;
use axum::Router;
use axum::body::{Body, Bytes};
use axum::extract::State;
use axum::http::{HeaderMap, StatusCode, header};
use axum::response::{IntoResponse, Response};
use futures_core::Stream;
use futures_util::{StreamExt, stream};
use log::{debug, error, info};
use serde::Serialize;
use serde_json::Value;
use tracing::{Instrument, Span};

use crate::agent_loop::{AgentLoopConfig, AgentLoopResult, run_agent_loop};
use crate::api::hosted_tools::strip_hosted_tools;
use crate::api::{self, EndpointKind};
use crate::llm_stream_mapper::{DefaultStreamMapperSelector, StreamMapperSelector};
use crate::metadata_mgr::{MetadataMgr, MetadataMgrImpl};
use crate::models::{ClientApi, ModelRequestNormalizerMap};
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
use crate::serve_config::ResponseModelMode;
use crate::tool_api::redacted_http_headers;
use crate::tool_invoker::ToolInvoker;
use crate::tool_mgr::ToolMgr;
use crate::trace::{DefaultRequestSpanStarter, RequestSpanStarter};
use crate::trace::{record_usage_attributes, set_http_span_status};
use crate::utils::{json_body_for_log, truncate_bytes_for_log};

#[derive(Clone)]
pub struct LlmHandlerState<A, M> {
    pub provider_registry: Arc<dyn ProviderRegistry<M>>,
    pub request_normalizers: Arc<ModelRequestNormalizerMap>,
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
    let request_value: Value = match serde_json::from_slice(&body) {
        Ok(value) => value,
        Err(err) => {
            let request_body = truncate_bytes_for_log(&body);
            let message = format!("invalid request body: {err}");
            error!(
                "chat request invalid json: {} status_code=400 trace_id={trace_id} body={request_body}",
                message
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

    let request_model = request_value
        .get("model")
        .and_then(Value::as_str)
        .unwrap_or_default();
    let request_model_id = if request_model.trim().is_empty() || request_model.trim() == "auto" {
        None
    } else {
        Some(request_model.to_owned())
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

    let request = match state.request_normalizers.normalize(
        ClientApi::ChatCompletions,
        &provider_entry.model_id,
        &request_value,
    ) {
        Ok(request) => request,
        Err(message) => return Ok(bad_request_response(&message)),
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

    let request_value: Value = match serde_json::from_slice(&body) {
        Ok(value) => value,
        Err(err) => {
            let response = bad_request_response(
                format!("invalid request body: {err}"),
                "invalid_request_error",
            );
            set_http_span_status(&root_span, response.status(), Some("invalid_request"));
            return Ok(response);
        }
    };

    let request_model = request_value
        .get("model")
        .and_then(Value::as_str)
        .unwrap_or_default();
    let request_model_id = if request_model.trim().is_empty() || request_model.trim() == "auto" {
        None
    } else {
        Some(request_model.to_owned())
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

    let request = match state.request_normalizers.normalize(
        ClientApi::Messages,
        &provider_entry.model_id,
        &request_value,
    ) {
        Ok(request) => request,
        Err(message) => return Ok(bad_request_response(message, "invalid_request_error")),
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

    let request_value: Value = match serde_json::from_slice(&body) {
        Ok(value) => value,
        Err(err) => {
            let message = format!("invalid request body: {err}");
            let response = bad_request_response(&message);
            set_http_span_status(&root_span, response.status(), Some("invalid_request"));
            return Ok(response);
        }
    };

    let request_model = request_value
        .get("model")
        .and_then(Value::as_str)
        .unwrap_or_default();
    let request_model_id = if request_model.trim().is_empty() || request_model.trim() == "auto" {
        None
    } else {
        Some(request_model.to_owned())
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

    let request = match state.request_normalizers.normalize(
        ClientApi::Responses,
        &provider_entry.model_id,
        &request_value,
    ) {
        Ok(request) => request,
        Err(message) => return Ok(bad_request_response(&message)),
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
    build_llm_api_with_normalizers(
        tool_mgr,
        provider_registry,
        tool_invoker,
        agent_loop_config,
        error_response_policy,
        Arc::new(ModelRequestNormalizerMap::default()),
    )
    .await
}

/// Build the conversation API with caller-provided model request normalizers.
pub async fn build_llm_api_with_request_normalizers(
    tool_mgr: Arc<dyn ToolMgr<(), ()>>,
    provider_registry: Arc<dyn crate::provider_registry::ProviderRegistry<()>>,
    tool_invoker: Arc<dyn ToolInvoker>,
    agent_loop_config: AgentLoopConfig<()>,
    request_normalizers: Arc<ModelRequestNormalizerMap>,
) -> anyhow::Result<Router> {
    build_llm_api_with_normalizers(
        tool_mgr,
        provider_registry,
        tool_invoker,
        agent_loop_config,
        Arc::new(DefaultLlmErrorResponsePolicy),
        request_normalizers,
    )
    .await
}

/// Build the conversation API with caller-provided model request normalizers.
pub async fn build_llm_api_with_normalizers(
    tool_mgr: Arc<dyn ToolMgr<(), ()>>,
    provider_registry: Arc<dyn crate::provider_registry::ProviderRegistry<()>>,
    tool_invoker: Arc<dyn ToolInvoker>,
    agent_loop_config: AgentLoopConfig<()>,
    error_response_policy: Arc<dyn LlmErrorResponsePolicy<()>>,
    request_normalizers: Arc<ModelRequestNormalizerMap>,
) -> anyhow::Result<Router> {
    let state = LlmHandlerState {
        provider_registry,
        request_normalizers,
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
        ) -> Result<UnifiedResponse, ProviderError> {
            Ok(self.response.clone())
        }

        async fn stream<'a>(
            &'a self,
            _request: UnifiedRequest,
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
            ) -> Result<UnifiedResponse, ProviderError> {
                *self.0.lock().await = Some(request);
                Ok(unified_response())
            }

            async fn stream<'a>(
                &'a self,
                request: UnifiedRequest,
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
