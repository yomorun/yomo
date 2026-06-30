use std::fmt;
use std::sync::Arc;

use anyhow::{Context, anyhow};
use axum::Router;
use axum::body::{Body, Bytes};
use axum::extract::State;
use axum::http::{HeaderMap, StatusCode, header};
use axum::response::{IntoResponse, Response};
use futures_util::{StreamExt, stream};
use log::{error, info};
use serde::Serialize;
use tracing::{Instrument, Span};

use crate::agent_loop::{AgentLoopConfig, AgentLoopResult, run_agent_loop};
use crate::llm_provider::FinishReason;
use crate::llm_provider::ProviderError;
use crate::llm_provider::registry::ProviderRegistry;
use crate::llm_provider::selection::SelectionError;
use crate::llm_stream_mapper::{DefaultStreamMapperSelector, StreamMapperSelector};
use crate::metadata_mgr::{MetadataMgr, MetadataMgrImpl};
use crate::openai_http_mapping::{
    map_chat_error, map_openai_response, map_usage_to_openai, openai_error_response,
    validate_openai_request,
};
use crate::openai_types::{ChatCompletionRequest, StreamOptions};
use crate::tool_invoker::ToolInvoker;
use crate::tool_mgr::ToolMgr;
use crate::trace::{DefaultRequestSpanStarter, RequestSpanStarter};
use crate::trace::{record_usage_attributes, set_http_span_status};
use crate::utils::truncate_bytes_for_log;

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

    let requested_model_is_default =
        request.model.trim().is_empty() || request.model.trim() == "auto";
    let request_model_id = if requested_model_is_default {
        state
            .provider_registry
            .default_model_id()
            .map(str::to_string)
    } else {
        Some(request.model.clone())
    };
    let provider_entry = match state
        .provider_registry
        .select(request_model_id.as_deref(), &metadata)
    {
        Ok(provider_entry) => provider_entry,
        Err(SelectionError::OutstandingBalance) => {
            let response = openai_error_response(
                StatusCode::PAYMENT_REQUIRED,
                "outstanding_balance",
                Some("outstanding_balance"),
            );
            set_http_span_status(&root_span, response.status(), Some("outstanding_balance"));
            return Ok(response);
        }
        Err(SelectionError::ModelNotSupported) => {
            let message = if requested_model_is_default && request_model_id.is_none() {
                "default model is not configured".to_string()
            } else {
                let model = request_model_id.as_deref().unwrap_or_default();
                format!("model {model} is not supported")
            };
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
        metadata_mgr: Arc::new(MetadataMgrImpl::new()),
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

#[cfg(test)]
mod tests {
    use axum::http::StatusCode;

    use super::trace_status_message_for_provider_error;
    use crate::llm_provider::ProviderError;
    use crate::openai_types::ErrorDetail;

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
}
