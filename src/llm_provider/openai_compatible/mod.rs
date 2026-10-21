use async_stream::try_stream;
use async_trait::async_trait;
use axum::http::StatusCode;
use futures_core::Stream;
use futures_util::StreamExt;
use log::warn;
use std::collections::HashMap;
use std::pin::Pin;
use std::sync::Arc;
use std::sync::atomic::{AtomicBool, Ordering};

use crate::llm_provider::openai_compatible::client::{ApiError, ClientError};
use crate::llm_provider::openai_responses::{
    ResponsesStreamState, UpstreamApiMode, can_fallback_to_chat_completions,
    chat_to_responses_request, is_responses_unsupported_status, map_responses_to_unified,
    map_stream_event, should_route_to_responses,
};
use crate::llm_provider::{Provider, ProviderError, UnifiedEvent, UnifiedResponse};
use crate::openai_http_mapping::validate_openai_request;
use crate::openai_types::{ChatCompletionRequest, Content, Message, Role};
use crate::serve_config::ConfigError;

pub mod client;

pub mod mapper;

const CONTENT_FILTER_MESSAGE: &str =
    "The request was rejected by the safety policy. Please revise your input and try again.";

#[derive(Clone)]
pub struct OpenAICompatibleProvider {
    client: client::Client,
    model_id: Option<String>,
    system_prompt: Option<String>,
    upstream_api: UpstreamApiMode,
    /// Set once the upstream rejected the Responses API, so later requests skip
    /// the failed Responses attempt.
    responses_unsupported: Arc<AtomicBool>,
}

impl OpenAICompatibleProvider {
    pub fn new(client: client::Client, model_id: Option<String>) -> Self {
        Self {
            client,
            model_id,
            system_prompt: None,
            upstream_api: UpstreamApiMode::default(),
            responses_unsupported: Arc::new(AtomicBool::new(false)),
        }
    }

    pub fn with_system_prompt(mut self, system_prompt: Option<String>) -> Self {
        self.system_prompt = system_prompt;
        self
    }

    /// Overrides how the upstream API is selected for a request.
    pub fn with_upstream_api(mut self, upstream_api: UpstreamApiMode) -> Self {
        self.upstream_api = upstream_api;
        self
    }

    /// Applies the provider-level model override and system prompt to a request
    /// before validation and dispatch.
    fn prepare_request(&self, request: &mut ChatCompletionRequest) -> Result<(), ProviderError> {
        if let Some(model_id) = &self.model_id {
            request.model = model_id.clone();
        }
        if let Some(system_prompt) = &self.system_prompt {
            prepend_system_prompt(request, system_prompt);
        }
        validate_request(request)
    }

    /// Returns `true` when the request must be sent to the Responses API.
    fn should_use_responses(&self, model: &str) -> bool {
        should_route_to_responses(
            self.upstream_api,
            self.responses_unsupported.load(Ordering::Relaxed),
            model,
        )
    }

    /// Returns `true` when a rejected Responses request may be replayed
    /// through Chat Completions.
    fn can_fallback(&self) -> bool {
        can_fallback_to_chat_completions(self.upstream_api)
    }
}

#[async_trait]
impl<M> Provider<M> for OpenAICompatibleProvider {
    fn model_id(&self) -> &str {
        "openai-compatible"
    }

    async fn complete(
        &self,
        mut request: ChatCompletionRequest,
        _metadata: &M,
    ) -> Result<UnifiedResponse, ProviderError> {
        self.prepare_request(&mut request)?;

        if self.should_use_responses(&request.model) {
            let translated = chat_to_responses_request(&request)?;
            match self.client.responses_completions(translated).await {
                Ok(response) => return map_responses_to_unified(response),
                // Unsupported upstream: replay through Chat Completions.
                Err(err) if self.can_fallback() && is_responses_unsupported(&err) => {
                    self.responses_unsupported.store(true, Ordering::Relaxed);
                    warn!("responses upstream unsupported, replaying via chat completions: {err}");
                }
                Err(err) => return Err(map_openai_error(err)),
            }
        }

        let response = self
            .client
            .chat_completions(request)
            .await
            .map_err(map_openai_error)?;

        mapper::map_response(response)
    }

    async fn stream<'a>(
        &'a self,
        mut request: ChatCompletionRequest,
        _metadata: &M,
    ) -> Result<
        Pin<Box<dyn Stream<Item = Result<UnifiedEvent, ProviderError>> + Send + 'a>>,
        ProviderError,
    > {
        self.prepare_request(&mut request)?;

        if self.should_use_responses(&request.model) {
            let translated = chat_to_responses_request(&request)?;
            match self.client.responses_completions_stream(translated).await {
                Ok(events) => return Ok(Box::pin(responses_event_stream(events))),
                // Unsupported upstream: replay through Chat Completions.
                Err(err) if self.can_fallback() && is_responses_unsupported(&err) => {
                    self.responses_unsupported.store(true, Ordering::Relaxed);
                    warn!("responses upstream unsupported, replaying via chat completions: {err}");
                }
                Err(err) => return Err(map_openai_error(err)),
            }
        }

        let stream = self
            .client
            .chat_completions_stream(request)
            .await
            .map_err(map_openai_error)?;

        let output = try_stream! {
            futures_util::pin_mut!(stream);
            let mut state = mapper::StreamMapState::default();

            while let Some(item) = stream.next().await {
                let chunk = item.map_err(map_openai_error)?;
                for event in mapper::map_stream_chunk(chunk, &mut state) {
                    yield event;
                }
            }
        };

        Ok(Box::pin(output))
    }
}

/// Returns `true` when the upstream rejected the Responses API itself rather
/// than the request content, so the request can be replayed via Chat
/// Completions.
pub(super) fn is_responses_unsupported(err: &ClientError) -> bool {
    let status = match err {
        ClientError::Api(ApiError::OpenAI { status, .. }) => Some(*status),
        ClientError::Api(ApiError::Unknown { status, .. }) => Some(*status),
        ClientError::Api(ApiError::Custom { status, .. }) => *status,
        _ => None,
    };
    status.is_some_and(is_responses_unsupported_status)
}

/// Wraps a raw Responses SSE data stream into unified events, closing after
/// the terminal completion event or an upstream error.
pub(super) fn responses_event_stream(
    events: impl Stream<Item = Result<String, ClientError>> + Send,
) -> impl Stream<Item = Result<UnifiedEvent, ProviderError>> {
    try_stream! {
        futures_util::pin_mut!(events);
        let mut state = ResponsesStreamState::default();

        while let Some(data) = events.next().await {
            let data = data.map_err(map_openai_error)?;
            for event in map_stream_event(&data, &mut state)? {
                let finished = matches!(
                    event,
                    UnifiedEvent::Completed { .. } | UnifiedEvent::Failed { .. }
                );
                yield event;
                if finished {
                    return;
                }
            }
        }
    }
}

fn map_openai_error(err: ClientError) -> ProviderError {
    match err {
        ClientError::Api(ApiError::OpenAI { status, mut error }) if status.as_u16() == 400 => {
            if error.code.as_deref() == Some("content_filter") {
                error.message = CONTENT_FILTER_MESSAGE.to_string();
            }
            ProviderError::Public {
                status: StatusCode::BAD_REQUEST,
                error,
            }
        }
        ClientError::Api(ApiError::OpenAI { status, error }) => {
            ProviderError::internal_with_upstream_status(status, error.message)
        }
        ClientError::Api(ApiError::Unknown { status, body }) => {
            ProviderError::internal_with_upstream_status(status, body)
        }
        other => ProviderError::internal(other.to_string()),
    }
}

pub fn build_openai_compatible_provider(
    params: &HashMap<String, String>,
) -> Result<OpenAICompatibleProvider, ConfigError> {
    let api_key = params.get("api_key").cloned().unwrap_or_default();
    let mut config = client::Config::new(api_key);
    let model_id = params.get("model").cloned();
    let system_prompt = params
        .get("system_prompt")
        .map(|prompt| prompt.trim().to_string())
        .filter(|prompt| !prompt.is_empty());
    let upstream_api = parse_upstream_api_mode(params.get("upstream_api"))?;
    if let Some(base_url) = params.get("base_url") {
        config = config.base_url(base_url.to_string());
    }
    let client =
        client::Client::new(config).map_err(|err| ConfigError::InvalidProvider(err.to_string()))?;
    Ok(OpenAICompatibleProvider::new(client, model_id)
        .with_system_prompt(system_prompt)
        .with_upstream_api(upstream_api))
}

/// Parses the optional `upstream_api` provider parameter.
///
/// Supported values are `auto` (default, selected from the model name),
/// `responses` and `chat_completions`.
pub(super) fn parse_upstream_api_mode(
    value: Option<&String>,
) -> Result<UpstreamApiMode, ConfigError> {
    let value = value.map(|value| value.trim().to_ascii_lowercase());
    match value.as_deref() {
        None | Some("") | Some("auto") => Ok(UpstreamApiMode::Auto),
        Some("responses") => Ok(UpstreamApiMode::Responses),
        Some("chat_completions") | Some("chat-completions") => Ok(UpstreamApiMode::ChatCompletions),
        Some(other) => Err(ConfigError::InvalidProvider(format!(
            "invalid upstream_api: {other}, expected auto, responses or chat_completions"
        ))),
    }
}

/// Prepends the provider-level system prompt to the request.
///
/// If the first message is already a system message with plain-text content,
/// the prompt is merged into its head as `"<system_prompt>\n\n<original>"` so
/// the request keeps a single leading system turn. Otherwise a new system
/// message is inserted at position 0.
fn prepend_system_prompt(request: &mut ChatCompletionRequest, system_prompt: &str) {
    if let Some(first) = request.messages.first_mut() {
        if first.role == Role::System {
            if let Content::Text(text) = &mut first.content {
                let merged = format!("{}\n\n{}", system_prompt, text.trim());
                first.content = Content::Text(merged);
                return;
            }
        }
    }
    request.messages.insert(
        0,
        Message {
            role: Role::System,
            content: Content::Text(system_prompt.to_string()),
            reasoning_content: None,
            tool_call_id: None,
            tool_calls: None,
        },
    );
}

fn validate_request(request: &ChatCompletionRequest) -> Result<(), ProviderError> {
    validate_openai_request(request).map_err(ProviderError::internal)
}

#[cfg(test)]
mod tests {
    use super::CONTENT_FILTER_MESSAGE;
    use super::build_openai_compatible_provider;
    use super::is_responses_unsupported;
    use super::map_openai_error;
    use super::prepend_system_prompt;
    use super::responses_event_stream;
    use futures_util::{StreamExt, pin_mut};
    use std::collections::HashMap;

    use crate::llm_provider::openai_compatible::client::{ApiError, ClientError};
    use crate::llm_provider::openai_responses::UpstreamApiMode;
    use crate::llm_provider::{ProviderError, UnifiedEvent};
    use crate::openai_types::ChatCompletionRequest;
    use crate::openai_types::{Content, ErrorDetail, Message, Role};
    use serde_json::Value;

    #[tokio::test]
    async fn responses_stream_stops_on_failure_after_tool_call() {
        let events = futures_util::stream::iter([
            Ok(r#"{"type":"response.output_item.done","item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"get_weather","arguments":"{}"}}"#.to_string()),
            Ok(r#"{"type":"response.failed","response":{"error":{"code":"server_error","message":"upstream unavailable"}}}"#.to_string()),
            Ok(r#"{"type":"response.completed","response":{"id":"resp_1","output":[]}}"#.to_string()),
        ]);
        let stream = responses_event_stream(events);
        pin_mut!(stream);

        assert!(matches!(
            stream.next().await,
            Some(Ok(UnifiedEvent::ToolCallDone { .. }))
        ));
        assert!(matches!(
            stream.next().await,
            Some(Err(ProviderError::Internal { .. }))
        ));
        assert!(stream.next().await.is_none());
    }

    #[test]
    fn responses_unsupported_only_matches_missing_endpoint_statuses() {
        let not_found = ClientError::Api(ApiError::Unknown {
            status: reqwest::StatusCode::NOT_FOUND,
            body: "not found".to_string(),
        });
        assert!(is_responses_unsupported(&not_found));

        let bad_request = ClientError::Api(ApiError::OpenAI {
            status: reqwest::StatusCode::BAD_REQUEST,
            error: ErrorDetail {
                message: "invalid".to_string(),
                r#type: "invalid_request_error".to_string(),
                code: None,
                param: None,
            },
        });
        assert!(!is_responses_unsupported(&bad_request));

        let custom = ClientError::Api(ApiError::Custom {
            status: Some(reqwest::StatusCode::NOT_FOUND),
            value: Value::Null,
        });
        assert!(is_responses_unsupported(&custom));

        let timeout = ClientError::Timeout("stream first byte timeout".to_string());
        assert!(!is_responses_unsupported(&timeout));
    }

    #[test]
    fn build_openai_compatible_provider_reads_upstream_api_param() {
        let params: HashMap<String, String> = HashMap::from([
            ("api_key".to_string(), "key".to_string()),
            ("upstream_api".to_string(), "responses".to_string()),
        ]);

        let provider = build_openai_compatible_provider(&params).expect("provider builds");

        assert_eq!(provider.upstream_api, UpstreamApiMode::Responses);
    }

    #[test]
    fn build_openai_compatible_provider_defaults_upstream_api_to_auto() {
        let params: HashMap<String, String> =
            HashMap::from([("api_key".to_string(), "key".to_string())]);

        let provider = build_openai_compatible_provider(&params).expect("provider builds");

        assert_eq!(provider.upstream_api, UpstreamApiMode::Auto);
    }

    #[test]
    fn build_openai_compatible_provider_rejects_invalid_upstream_api() {
        let params: HashMap<String, String> = HashMap::from([
            ("api_key".to_string(), "key".to_string()),
            ("upstream_api".to_string(), "grpc".to_string()),
        ]);

        let error = match build_openai_compatible_provider(&params) {
            Ok(_) => panic!("expected invalid upstream_api to be rejected"),
            Err(error) => error,
        };

        assert!(error.to_string().contains("invalid upstream_api"));
    }

    #[test]
    fn map_openai_error_rewrites_content_filter_message() {
        let err = ClientError::Api(ApiError::OpenAI {
            status: reqwest::StatusCode::BAD_REQUEST,
            error: ErrorDetail {
                message: "upstream message".to_string(),
                r#type: "invalid_request_error".to_string(),
                code: Some("content_filter".to_string()),
                param: None,
            },
        });

        let mapped = map_openai_error(err);

        let ProviderError::Public { status, error } = mapped else {
            panic!("expected public error");
        };
        assert_eq!(status, axum::http::StatusCode::BAD_REQUEST);
        assert_eq!(error.message, CONTENT_FILTER_MESSAGE);
    }

    #[test]
    fn map_openai_error_keeps_non_filter_bad_request_message() {
        let err = ClientError::Api(ApiError::OpenAI {
            status: reqwest::StatusCode::BAD_REQUEST,
            error: ErrorDetail {
                message: "original".to_string(),
                r#type: "invalid_request_error".to_string(),
                code: Some("invalid_parameter".to_string()),
                param: None,
            },
        });

        let mapped = map_openai_error(err);

        let ProviderError::Public { error, .. } = mapped else {
            panic!("expected public error");
        };
        assert_eq!(error.message, "original");
    }

    fn text_message(role: Role, text: &str) -> Message {
        Message {
            role,
            content: Content::Text(text.to_string()),
            reasoning_content: None,
            tool_call_id: None,
            tool_calls: None,
        }
    }

    fn request_with_messages(messages: Vec<Message>) -> ChatCompletionRequest {
        ChatCompletionRequest {
            model: "viv-fast".to_string(),
            messages,
            ..Default::default()
        }
    }

    #[test]
    fn prepend_system_prompt_inserts_when_no_system_message() {
        let mut request = request_with_messages(vec![text_message(Role::User, "who are you?")]);

        prepend_system_prompt(&mut request, "You are viv-fast.");

        assert_eq!(request.messages.len(), 2);
        assert_eq!(request.messages[0].role, Role::System);
        assert_eq!(
            request.messages[0].content,
            Content::Text("You are viv-fast.".to_string())
        );
        assert_eq!(request.messages[1].role, Role::User);
    }

    #[test]
    fn prepend_system_prompt_merges_into_leading_system_message() {
        let mut request = request_with_messages(vec![
            text_message(Role::System, "Be helpful."),
            text_message(Role::User, "hi"),
        ]);

        prepend_system_prompt(&mut request, "You are viv-fast.");

        assert_eq!(request.messages.len(), 2);
        assert_eq!(request.messages[0].role, Role::System);
        assert_eq!(
            request.messages[0].content,
            Content::Text("You are viv-fast.\n\nBe helpful.".to_string())
        );
        assert_eq!(request.messages[1].role, Role::User);
    }

    #[test]
    fn prepend_system_prompt_inserts_when_first_message_is_user() {
        let mut request = request_with_messages(vec![
            text_message(Role::User, "hello"),
            text_message(Role::System, "later system"),
            text_message(Role::User, "hi"),
        ]);

        prepend_system_prompt(&mut request, "You are viv-fast.");

        assert_eq!(request.messages.len(), 4);
        assert_eq!(request.messages[0].role, Role::System);
        assert_eq!(
            request.messages[0].content,
            Content::Text("You are viv-fast.".to_string())
        );
        assert_eq!(request.messages[1].role, Role::User);
        // The existing system message stays untouched because it is not leading.
        assert_eq!(
            request.messages[2].content,
            Content::Text("later system".to_string())
        );
    }

    #[test]
    fn prepend_system_prompt_merges_and_trims_existing_system_text() {
        let mut request =
            request_with_messages(vec![text_message(Role::System, "  Be helpful.  ")]);

        prepend_system_prompt(&mut request, "You are viv-fast.");

        assert_eq!(request.messages.len(), 1);
        assert_eq!(
            request.messages[0].content,
            Content::Text("You are viv-fast.\n\nBe helpful.".to_string())
        );
    }

    #[test]
    fn prepend_system_prompt_inserts_before_parts_content_system_message() {
        use crate::openai_types::ContentPart;

        let mut request = request_with_messages(vec![Message {
            role: Role::System,
            content: Content::Parts(vec![ContentPart::Text {
                text: "structured".to_string(),
            }]),
            reasoning_content: None,
            tool_call_id: None,
            tool_calls: None,
        }]);

        prepend_system_prompt(&mut request, "You are viv-fast.");

        assert_eq!(request.messages.len(), 2);
        assert_eq!(
            request.messages[0].content,
            Content::Text("You are viv-fast.".to_string())
        );
        assert_eq!(request.messages[1].role, Role::System);
    }

    #[test]
    fn build_openai_compatible_provider_reads_system_prompt_from_params() {
        let params: HashMap<String, String> = HashMap::from([
            ("api_key".to_string(), "key".to_string()),
            ("system_prompt".to_string(), "You are viv-fast.".to_string()),
        ]);

        let provider = build_openai_compatible_provider(&params).expect("provider builds");

        assert_eq!(
            provider.system_prompt,
            Some("You are viv-fast.".to_string())
        );
    }

    #[test]
    fn build_openai_compatible_provider_skips_blank_system_prompt() {
        let params: HashMap<String, String> = HashMap::from([
            ("api_key".to_string(), "key".to_string()),
            ("system_prompt".to_string(), "   ".to_string()),
        ]);

        let provider = build_openai_compatible_provider(&params).expect("provider builds");

        assert_eq!(provider.system_prompt, None);
    }
}
