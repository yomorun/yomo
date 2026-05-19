use std::fmt;
use std::sync::Arc;

use anyhow::Context;
use axum::Router;
use axum::body::{Body, Bytes};
use axum::extract::State;
use axum::http::{HeaderMap, StatusCode, header};
use axum::response::{IntoResponse, Response};
use log::{error, info};
use serde::Serialize;
use serde_json::Value;
use tracing::{Instrument, Span};

use crate::agent_loop::{AgentLoopConfig, AgentLoopResult, run_agent_loop};
use crate::llm_provider::FinishReason;
use crate::llm_provider::registry::ProviderRegistry;
use crate::llm_provider::selection::SelectionError;
use crate::llm_stream_mapper::{DefaultStreamMapperSelector, StreamMapperSelector};
use crate::metadata_mgr::MetadataMgr;
use crate::openai_http_mapping::{
    map_chat_error, map_openai_response, openai_error_response, validate_openai_request,
};
use crate::openai_types::ChatCompletionRequest;
use crate::tool_invoker::ToolInvoker;
use crate::tool_mgr::ToolMgr;
use crate::trace::{DefaultRequestSpanStarter, RequestSpanStarter};
use crate::trace::{record_flattened_json_attributes, set_http_span_status};

#[derive(Clone)]
pub struct LlmHandlerState<A, M> {
    pub provider_registry: Arc<ProviderRegistry<M>>,
    pub tool_mgr: Arc<dyn ToolMgr<A, M>>,
    pub tool_invoker: Arc<dyn ToolInvoker>,
    pub metadata_mgr: Arc<dyn MetadataMgr<A, M>>,
    pub request_span_starter: Arc<dyn RequestSpanStarter<M>>,
    pub agent_loop_config: AgentLoopConfig<M>,
    pub mapper_selector: Arc<dyn StreamMapperSelector>,
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

    let (response, status_message) = match handle_chat_completions_inner::<A, M>(
        state,
        metadata.to_owned(),
        trace_id,
        body,
        root_span.clone(),
        headers.clone(),
    )
    .instrument(root_span.clone())
    .await
    {
        Ok(response) => (response, None),
        Err(err) => {
            error!("chat completion failed: {err} {:?}", metadata);
            let message = err.to_string();
            (
                openai_error_response(StatusCode::INTERNAL_SERVER_ERROR, "internal error", None),
                Some(message),
            )
        }
    };
    set_http_span_status(&root_span, response.status(), status_message.as_deref());
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
    let mut request: ChatCompletionRequest = match serde_json::from_slice(&body) {
        Ok(request) => request,
        Err(err) => {
            error!("chat request invalid json: {err} trace_id={trace_id}");
            return Ok(openai_error_response(
                StatusCode::BAD_REQUEST,
                &format!("invalid request body: {err}"),
                Some("invalid_request_error"),
            ));
        }
    };

    let server_tools = state
        .tool_mgr
        .list_tools(&metadata)
        .await
        .map_err(|err| anyhow::anyhow!("tool manager error: {err}"))?;

    if let Err(err) = state
        .agent_loop_config
        .request_hook
        .preprocess(&trace_id, &metadata, &mut request)
        .await
    {
        error!("request preprocess failed: {err}");
        return Ok(openai_error_response(
            StatusCode::BAD_REQUEST,
            &err.to_string(),
            Some("invalid_request_error"),
        ));
    }

    let request_model_id = if request.model.trim().is_empty() {
        None
    } else {
        Some(request.model.clone())
    };
    let provider_entry = match state
        .provider_registry
        .select(request_model_id.as_deref(), &metadata)
    {
        Ok(provider_entry) => provider_entry,
        Err(SelectionError::OutstandingBalance) => {
            return Ok(openai_error_response(
                StatusCode::PAYMENT_REQUIRED,
                "outstanding_balance",
                Some("outstanding_balance"),
            ));
        }
        Err(SelectionError::ModelNotSupported) => {
            let model = request_model_id.as_deref().unwrap_or("");
            let message = format!("model {model} is not supported");
            return Ok(openai_error_response(
                StatusCode::BAD_REQUEST,
                &message,
                Some("invalid_request_error"),
            ));
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
                request.stream_options = Some(crate::openai_types::StreamOptions {
                    include_usage: true,
                    include_obfuscation: None,
                });
            }
        }
    }
    info!(
        "http.request.start; method=POST path=/v1/chat/completion model_id={} stream={} trace_id={} metadata={:?}",
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
        return Ok(openai_error_response(
            StatusCode::BAD_REQUEST,
            &message,
            Some("invalid_request_error"),
        ));
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
            record_flattened_json_attributes(&root_span, "usage", &usage_to_value(&response.usage));
            info!(
                "http.request.end; status_code=200 model_id={} prompt_tokens={} completion_tokens={} trace_id={} metadata={:?}",
                model_id,
                response.usage.input_tokens,
                response.usage.output_tokens,
                trace_id,
                metadata
            );
            let mapped = map_openai_response(response);
            let payload = serde_json::to_vec(&mapped).context("serialize response")?;
            Ok(Response::builder()
                .status(StatusCode::OK)
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(payload))
                .expect("build response"))
        }
        Ok(AgentLoopResult::Stream { events }) => {
            let mapper = state.mapper_selector.select(&headers);
            let sse = mapper.map_stream(events, trace_id, model_id, root_span.clone());
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
            error!(
                "http.request.end; status_code=500 model_id={} error={} trace_id={} metadata={:?}",
                model_id, err, trace_id, metadata
            );
            let response = map_chat_error(err);
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

fn usage_to_value(usage: &crate::llm_provider::Usage) -> Value {
    let mut usage = usage.clone();
    usage.raw = None;
    serde_json::to_value(usage).unwrap_or(Value::Null)
}

pub async fn build_llm_api(
    tool_mgr: Arc<dyn ToolMgr<(), ()>>,
    provider_registry: ProviderRegistry<()>,
    tool_invoker: Arc<dyn ToolInvoker>,
    agent_loop_config: AgentLoopConfig<()>,
) -> anyhow::Result<Router> {
    let state = LlmHandlerState {
        provider_registry: Arc::new(provider_registry),
        tool_mgr,
        tool_invoker,
        metadata_mgr: Arc::new(crate::metadata_mgr::MetadataMgrImpl::new()),
        request_span_starter: Arc::new(DefaultRequestSpanStarter),
        agent_loop_config,
        mapper_selector: Arc::new(DefaultStreamMapperSelector::default()),
    };

    let app = axum::Router::new()
        .route(
            "/chat/completions",
            axum::routing::post(handle_chat_completions::<(), ()>),
        )
        .with_state(state);
    Ok(app)
}
