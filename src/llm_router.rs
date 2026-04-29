use std::fmt;

use anyhow::Context;
use axum::body::{Body, Bytes};
use axum::extract::State;
use axum::http::{HeaderMap, StatusCode, header};
use axum::response::{IntoResponse, Response};
use log::{error, info};
use opentelemetry::trace::TraceContextExt;
use opentelemetry_sdk::trace::{IdGenerator, RandomIdGenerator};
use tracing::{Instrument, Span, debug_span};
use tracing_opentelemetry::OpenTelemetrySpanExt;

use crate::agent_loop::{AgentLoopConfig, AgentLoopResult, run_agent_loop};
use crate::llm_providers::registry::ProviderRegistry;
use crate::llm_providers::selection::SelectionError;
use crate::openai_http_mapping::{
    map_chat_error, map_openai_response, openai_error_response, stream_openai_chunks,
    validate_openai_request,
};
use crate::openai_types::ChatCompletionRequest;
use crate::tool_invoker::ToolInvoker;
use crate::auth::Auth;
use crate::metadata_mgr::MetadataMgr;
use crate::tool_mgr::ToolMgr;
use axum::Router;
use std::sync::Arc;

pub struct LlmHandlerState<A, M> {
    pub provider_registry: std::sync::Arc<ProviderRegistry<M>>,
    pub tool_mgr: std::sync::Arc<dyn ToolMgr<A, M>>,
    pub tool_invoker: std::sync::Arc<dyn ToolInvoker<M>>,
    pub metadata_mgr: std::sync::Arc<dyn MetadataMgr<A, M>>,
    pub auth: std::sync::Arc<dyn Auth<A>>,
}

impl<A, M> Clone for LlmHandlerState<A, M> {
    fn clone(&self) -> Self {
        Self {
            provider_registry: std::sync::Arc::clone(&self.provider_registry),
            tool_mgr: std::sync::Arc::clone(&self.tool_mgr),
            tool_invoker: std::sync::Arc::clone(&self.tool_invoker),
            metadata_mgr: std::sync::Arc::clone(&self.metadata_mgr),
            auth: std::sync::Arc::clone(&self.auth),
        }
    }
}

/// Parse HTTP header value
fn parse_http_headers(http_headers: &HeaderMap, key: &str) -> String {
    http_headers
        .get(key)
        .and_then(|value| value.to_str().ok())
        .unwrap_or_default()
        .to_string()
}

fn parse_credential(http_headers: &HeaderMap) -> String {
    let credential = parse_http_headers(http_headers, "X-Credential");
    if !credential.trim().is_empty() {
        return credential;
    }
    let auth_header = parse_http_headers(http_headers, "Authorization");
    let bearer_prefix = "Bearer ";
    if let Some(token) = auth_header.strip_prefix(bearer_prefix) {
        return token.trim().to_string();
    }
    String::new()
}

pub async fn handle_chat_completions<A, M>(
    State(state): State<LlmHandlerState<A, M>>,
    headers: HeaderMap,
    body: Bytes,
) -> impl IntoResponse
where
    A: Send + Sync + 'static,
    M: Send + Sync + fmt::Debug + Clone + 'static,
{
    let root_span = debug_span!(
        "http.request",
        http.method = "POST",
        http.route = "/v1/chat/completions",
    );
    let trace_id = {
        let span_context = root_span.context().span().span_context().clone();
        if span_context.is_valid() {
            span_context.trace_id().to_string()
        } else {
            RandomIdGenerator::default().new_trace_id().to_string()
        }
    };
    let extension = String::new();
    let credential = parse_credential(&headers);

    let auth_info = match state.auth.authenticate(&credential).await {
        Ok(info) => info,
        Err(err) => {
            error!("chat auth failed: {err}");
            return openai_error_response(
                StatusCode::UNAUTHORIZED,
                "unauthorized",
                Some("invalid_request_error"),
            );
        }
    };

    let metadata = match state.metadata_mgr.new_from_extension(&auth_info, &extension) {
        Ok(metadata) => metadata,
        Err(err) => {
            error!("metadata init failed: {err}");
            return openai_error_response(
                StatusCode::INTERNAL_SERVER_ERROR,
                "metadata error",
                None,
            );
        }
    };

    let metadata_for_error = metadata.clone();
    match handle_chat_completions_inner::<A, M>(
        state,
        metadata,
        trace_id,
        body,
        extension,
        root_span.clone(),
    )
    .instrument(root_span.clone())
    .await
    {
        Ok(response) => response,
        Err(err) => {
            error!("chat completion failed: {err} {:?}", metadata_for_error);
            openai_error_response(StatusCode::INTERNAL_SERVER_ERROR, "internal error", None)
        }
    }
}

async fn handle_chat_completions_inner<A, M>(
    state: LlmHandlerState<A, M>,
    metadata: M,
    trace_id: String,
    body: Bytes,
    extension: String,
    root_span: Span,
) -> Result<Response, anyhow::Error>
where
    A: Send + Sync + 'static,
    M: fmt::Debug + Clone + Send + Sync + 'static,
{
    let mut request: ChatCompletionRequest = match serde_json::from_slice(&body) {
        Ok(request) => request,
        Err(err) => {
            error!("chat request invalid json: {err}");
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

    let request_model_id = if request.model.trim().is_empty() {
        None
    } else {
        Some(request.model.clone())
    };
    let (selection, provider_entry) = match state
        .provider_registry
        .select(request_model_id.as_deref(), &metadata)
    {
        Ok(selection) => selection,
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

    request.model = selection.model_id.clone();
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
        request_model_id.as_deref().unwrap_or(&selection.model_id),
        stream,
        trace_id,
        metadata
    );
    if let Err(message) = validate_openai_request(&request) {
        error!(
            "chat request invalid: model_id={}, error={} {:?}",
            request_model_id.as_deref().unwrap_or(&selection.model_id),
            message,
            metadata
        );
        return Ok(openai_error_response(
            StatusCode::BAD_REQUEST,
            &message,
            Some("invalid_request_error"),
        ));
    }

    let provider = std::sync::Arc::clone(&provider_entry.provider);

    let model_for_log = selection.model_id.clone();
    let request_model = selection.model_id.clone();
    let metadata_for_log = metadata.clone();
    let loop_result = run_agent_loop::<A, M>(
        provider,
        request,
        server_tools,
        std::sync::Arc::clone(&state.tool_invoker),
        metadata,
        trace_id.clone(),
        extension,
        AgentLoopConfig::default(),
    )
    .await;

    match loop_result {
        Ok(AgentLoopResult::NonStream(response)) => {
            info!(
                "http.request.end; status_code=200 model_id={} prompt_tokens={} completion_tokens={} trace_id={} metadata={:?}",
                request_model,
                response.usage.input_tokens,
                response.usage.output_tokens,
                trace_id,
                metadata_for_log
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
            let sse = stream_openai_chunks(events, trace_id, request_model, root_span.clone());
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
                model_for_log,
                err,
                trace_id,
                metadata_for_log
            );
            let response = map_chat_error(err);
            Ok(response)
        }
    }
}

pub async fn build_llm_router(
    tool_mgr: Arc<dyn ToolMgr<(), ()>>,
    provider_registry: ProviderRegistry<()>,
    tool_invoker: Arc<dyn ToolInvoker<()>>,
) -> anyhow::Result<Router> {
    let state = LlmHandlerState {
        provider_registry: Arc::new(provider_registry),
        tool_mgr,
        tool_invoker,
        metadata_mgr: Arc::new(crate::metadata_mgr::MetadataMgrImpl::new()),
        auth: Arc::new(crate::auth::AuthImpl::new(None)),
    };

    let app = axum::Router::new()
        .route("/chat/completions", axum::routing::post(handle_chat_completions::<(), ()>))
        .with_state(state);
    Ok(app)
}
