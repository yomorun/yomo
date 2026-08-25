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
use crate::llm_stream_mapper::{DefaultStreamMapperSelector, StreamMapperSelector};
use crate::metadata_mgr::{MetadataMgr, MetadataMgrImpl};
use crate::openai_http_mapping::{
    INTERNAL_SERVER_ERROR_MESSAGE, map_openai_response, map_usage_to_openai, openai_error_response,
    validate_openai_request,
};
use crate::openai_types::{ChatCompletionRequest, StreamOptions};
use crate::provider_registry::{ProviderRegistry, SelectionError};
use crate::serve_config::EndpointKind;
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

fn finish_reason_to_str(reason: &FinishReason) -> &'static str {
    match reason {
        FinishReason::Stop => "stop",
        FinishReason::Length => "length",
        FinishReason::ToolCalls => "tool_calls",
        FinishReason::ContentFilter => "content_filter",
        FinishReason::Other => "other",
    }
}

fn provider_error_status(err: &ProviderError) -> StatusCode {
    match err {
        ProviderError::Public { status, .. } if *status == StatusCode::BAD_REQUEST => *status,
        ProviderError::Public { .. } => StatusCode::INTERNAL_SERVER_ERROR,
        ProviderError::Internal { .. } => StatusCode::INTERNAL_SERVER_ERROR,
    }
}

fn selection_model_required_message(endpoint: &str) -> String {
    format!("model is required for {endpoint}")
}

fn selection_model_not_supported_message(endpoint: &str, model: &str) -> String {
    format!("model {model} is not supported for {endpoint}")
}

pub async fn build_llm_api(
    tool_mgr: Arc<dyn ToolMgr<(), ()>>,
    provider_registry: ProviderRegistry<()>,
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
    provider_registry: ProviderRegistry<()>,
    tool_invoker: Arc<dyn ToolInvoker>,
    agent_loop_config: AgentLoopConfig<()>,
    error_response_policy: Arc<dyn LlmErrorResponsePolicy<()>>,
) -> anyhow::Result<Router> {
    let state = LlmHandlerState {
        provider_registry: Arc::new(provider_registry),
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
        .with_state(state);
    Ok(app)
}

#[cfg(test)]
mod tests {
    use axum::http::StatusCode;
    use axum::response::Response;

    use super::{
        DefaultLlmErrorResponsePolicy, LlmErrorResponsePolicy, provider_error_status,
        selection_model_not_supported_message, selection_model_required_message,
    };
    use crate::llm_provider::ProviderError;
    use crate::openai_types::ErrorDetail;

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
    fn provider_error_status_maps_non_bad_request_to_internal_server_error() {
        let err = ProviderError::Public {
            status: StatusCode::TOO_MANY_REQUESTS,
            error: ErrorDetail {
                message: "rate limited".to_string(),
                r#type: "rate_limit_error".to_string(),
                code: Some("rate_limit".to_string()),
                param: None,
            },
        };

        assert_eq!(
            provider_error_status(&err),
            StatusCode::INTERNAL_SERVER_ERROR
        );
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
    fn selection_model_not_supported_message_includes_endpoint() {
        assert_eq!(
            selection_model_not_supported_message("/v1/chat/completions", "gpt-x"),
            "model gpt-x is not supported for /v1/chat/completions"
        );
    }
}
