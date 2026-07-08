use async_stream::try_stream;
use async_trait::async_trait;
use axum::http::StatusCode;
use futures_core::Stream;
use futures_util::StreamExt;
use serde::Deserialize;
use serde_json::Value;
use std::collections::HashMap;
use std::pin::Pin;

use crate::llm_provider::openai_compatible::client::{ApiError, ClientError};
use crate::llm_provider::openai_compatible::{client, mapper};
use crate::llm_provider::{Provider, ProviderError, UnifiedEvent, UnifiedResponse};
use crate::openai_http_mapping::validate_openai_request;
use crate::openai_types::{ChatCompletionRequest, Message, Role, ThinkingConfig, ThinkingType};
use crate::serve_config::ConfigError;

#[derive(Clone)]
pub struct TokenHubProvider {
    client: client::Client,
    model_id: Option<String>,
}

#[derive(Debug, Deserialize)]
struct TokenHubErrorEnvelope {
    error: TokenHubErrorDetail,
}

#[derive(Debug, Deserialize)]
struct TokenHubErrorDetail {
    #[serde(rename = "type")]
    r#type: Option<String>,
    code: Option<String>,
    message: Option<String>,
    source: Option<String>,
    upstream_code: Option<String>,
    upstream_status: Option<u16>,
    request_id: Option<String>,
}

impl TokenHubProvider {
    pub fn new(client: client::Client, model_id: Option<String>) -> Self {
        Self { client, model_id }
    }
}

#[async_trait]
impl<M> Provider<M> for TokenHubProvider {
    fn model_id(&self) -> &str {
        "tokenhub"
    }

    async fn complete(
        &self,
        mut request: ChatCompletionRequest,
        _metadata: &M,
    ) -> Result<UnifiedResponse, ProviderError> {
        if let Some(model_id) = &self.model_id {
            request.model = model_id.clone();
        }

        normalize_tokenhub_request(&mut request);
        validate_request(&request)?;

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
        if let Some(model_id) = &self.model_id {
            request.model = model_id.clone();
        }

        normalize_tokenhub_request(&mut request);
        validate_request(&request)?;

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

pub fn build_tokenhub_provider(
    params: &HashMap<String, String>,
) -> Result<TokenHubProvider, ConfigError> {
    let api_key = params
        .get("api_key")
        .ok_or_else(|| ConfigError::InvalidProvider("api_key is required".to_string()))?;

    let mut config = client::Config::new(api_key.to_string());
    let model_id = params.get("model").cloned();
    if let Some(base_url) = params.get("base_url") {
        config = config.base_url(base_url.to_string());
    } else {
        config = config.base_url("https://tokenhub.tencentmaas.com/v1".to_string());
    }
    config = config.error_parser(parse_tokenhub_error_payload);

    let client =
        client::Client::new(config).map_err(|err| ConfigError::InvalidProvider(err.to_string()))?;
    Ok(TokenHubProvider::new(client, model_id))
}

fn validate_request(request: &ChatCompletionRequest) -> Result<(), ProviderError> {
    validate_openai_request(request).map_err(ProviderError::internal)
}

pub(crate) fn normalize_tokenhub_request(request: &mut ChatCompletionRequest) {
    normalize_default_thinking(request);
    normalize_reasoning_effort(request);
    normalize_reasoning_content_for_tool_calls(request);
}

fn normalize_default_thinking(request: &mut ChatCompletionRequest) {
    if request.thinking.is_none() {
        let thinking_type = preferred_thinking_type_for_model(&request.model);
        request.thinking = Some(ThinkingConfig {
            kind: thinking_type,
        });
    }
}

fn normalize_reasoning_effort(request: &mut ChatCompletionRequest) {
    let effort = request.reasoning_effort.take();
    let Some(effort) = effort else {
        return;
    };

    let thinking_type = preferred_thinking_type_for_model(&request.model);
    request.thinking = Some(ThinkingConfig {
        kind: thinking_type,
    });

    request.reasoning_effort = Some(map_reasoning_effort_to_tokenhub(&effort).to_string());
}

fn preferred_thinking_type_for_model(model: &str) -> ThinkingType {
    // TokenHub OpenAI-compatible API doc:
    // https://cloud.tencent.com/document/product/1823/130079
    // minimax-m3 rejects `thinking.type = enabled` and only accepts adaptive/disabled.
    if model == "minimax-m3" {
        ThinkingType::Adaptive
    } else {
        ThinkingType::Enabled
    }
}

fn map_reasoning_effort_to_tokenhub(effort: &str) -> &str {
    match effort {
        "minimal" => "low",
        "low" => "low",
        "medium" => "medium",
        "high" => "high",
        "max" => "high",
        other => other,
    }
}

fn normalize_reasoning_content_for_tool_calls(request: &mut ChatCompletionRequest) {
    let thinking_enabled = matches!(
        request.thinking.as_ref().map(|value| &value.kind),
        Some(ThinkingType::Enabled | ThinkingType::Adaptive)
    );
    if !thinking_enabled {
        return;
    }

    for message in &mut request.messages {
        ensure_reasoning_content_for_assistant_tool_call(message);
    }
}

fn ensure_reasoning_content_for_assistant_tool_call(message: &mut Message) {
    if message.role != Role::Assistant {
        return;
    }

    let has_tool_calls = message
        .tool_calls
        .as_ref()
        .map(|calls| !calls.is_empty())
        .unwrap_or(false);
    if !has_tool_calls {
        return;
    }

    let missing_reasoning_content = message
        .reasoning_content
        .as_deref()
        .map(str::trim)
        .map(|value| value.is_empty())
        .unwrap_or(true);
    if missing_reasoning_content {
        message.reasoning_content = Some(" ".to_string());
    }
}

fn map_openai_error(err: ClientError) -> ProviderError {
    match err {
        ClientError::Api(ApiError::OpenAI { status, error }) if status.as_u16() == 400 => {
            ProviderError::Public {
                status: StatusCode::BAD_REQUEST,
                error,
            }
        }
        ClientError::Api(ApiError::Custom { status, value }) => {
            if let Some(error) = map_tokenhub_custom_error(value) {
                let status = status.unwrap_or(StatusCode::BAD_GATEWAY);
                ProviderError::Public { status, error }
            } else {
                ProviderError::internal("tokenhub custom error parse failed")
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

fn parse_tokenhub_error_payload(body: &[u8]) -> Option<Value> {
    let value = serde_json::from_slice::<Value>(body).ok()?;
    serde_json::from_value::<TokenHubErrorEnvelope>(value.clone()).ok()?;
    Some(value)
}

fn map_tokenhub_custom_error(value: Value) -> Option<crate::openai_types::ErrorDetail> {
    let envelope = serde_json::from_value::<TokenHubErrorEnvelope>(value).ok()?;
    let detail = envelope.error;
    let message = detail.message.unwrap_or_default();

    let metadata = [
        detail
            .source
            .as_deref()
            .map(|value| format!("source={value}")),
        detail
            .upstream_code
            .as_deref()
            .map(|value| format!("upstream_code={value}")),
        detail
            .upstream_status
            .map(|value| format!("upstream_status={value}")),
        detail
            .request_id
            .as_deref()
            .map(|value| format!("request_id={value}")),
    ]
    .into_iter()
    .flatten()
    .collect::<Vec<_>>();

    let message = if metadata.is_empty() {
        message
    } else {
        format!("{} [{}]", message, metadata.join(" "))
    };

    let error = crate::openai_types::ErrorDetail {
        message,
        r#type: detail.r#type.unwrap_or_default(),
        code: detail.code,
        param: None,
    };

    Some(error)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::llm_provider::openai_compatible::client::{ApiError, ClientError};
    use crate::openai_types::{Content, ToolCall, ToolCallFunction};

    fn request_with_messages(messages: Vec<Message>) -> ChatCompletionRequest {
        ChatCompletionRequest {
            model: "kimi-k2.6".to_string(),
            messages,
            n: None,
            temperature: None,
            top_p: None,
            presence_penalty: None,
            frequency_penalty: None,
            logprobs: None,
            top_logprobs: None,
            modalities: None,
            audio: None,
            max_completion_tokens: None,
            stop: None,
            response_format: None,
            thinking: None,
            reasoning_effort: None,
            chat_template_kwargs: None,
            prediction: None,
            verbosity: None,
            tools: None,
            tool_choice: None,
            allowed_tools: None,
            parallel_tool_calls: None,
            service_tier: None,
            seed: None,
            stream: None,
            stream_options: None,
            metadata: None,
            agent_context: None,
        }
    }

    fn assistant_tool_call_message(reasoning_content: Option<&str>) -> Message {
        Message {
            role: Role::Assistant,
            content: Content::Text(String::new()),
            reasoning_content: reasoning_content.map(|value| value.to_string()),
            tool_call_id: None,
            tool_calls: Some(vec![ToolCall {
                id: Some("call_1".to_string()),
                r#type: Some("function".to_string()),
                function: ToolCallFunction {
                    name: "search".to_string(),
                    arguments: "{}".to_string(),
                    description: None,
                },
            }]),
        }
    }

    #[test]
    fn normalize_reasoning_effort_maps_and_enables_thinking() {
        let mut request = request_with_messages(vec![Message {
            role: Role::User,
            content: Content::Text("hello".to_string()),
            reasoning_content: None,
            tool_call_id: None,
            tool_calls: None,
        }]);
        request.reasoning_effort = Some("max".to_string());

        normalize_tokenhub_request(&mut request);

        assert!(matches!(
            request.thinking.as_ref().map(|value| &value.kind),
            Some(ThinkingType::Enabled)
        ));
        assert_eq!(request.reasoning_effort.as_deref(), Some("high"));
    }

    #[test]
    fn normalize_enables_thinking_by_default() {
        let mut request = request_with_messages(vec![Message {
            role: Role::User,
            content: Content::Text("hello".to_string()),
            reasoning_content: None,
            tool_call_id: None,
            tool_calls: None,
        }]);

        normalize_tokenhub_request(&mut request);

        assert!(matches!(
            request.thinking.as_ref().map(|value| &value.kind),
            Some(ThinkingType::Enabled)
        ));
    }

    #[test]
    fn normalize_uses_adaptive_thinking_for_minimax_m3() {
        let mut request = request_with_messages(vec![Message {
            role: Role::User,
            content: Content::Text("hello".to_string()),
            reasoning_content: None,
            tool_call_id: None,
            tool_calls: None,
        }]);
        request.model = "minimax-m3".to_string();

        normalize_tokenhub_request(&mut request);

        assert!(matches!(
            request.thinking.as_ref().map(|value| &value.kind),
            Some(ThinkingType::Adaptive)
        ));
    }

    #[test]
    fn normalize_reasoning_effort_uses_adaptive_thinking_for_minimax_m3() {
        let mut request = request_with_messages(vec![Message {
            role: Role::User,
            content: Content::Text("hello".to_string()),
            reasoning_content: None,
            tool_call_id: None,
            tool_calls: None,
        }]);
        request.model = "minimax-m3".to_string();
        request.reasoning_effort = Some("max".to_string());

        normalize_tokenhub_request(&mut request);

        assert!(matches!(
            request.thinking.as_ref().map(|value| &value.kind),
            Some(ThinkingType::Adaptive)
        ));
        assert_eq!(request.reasoning_effort.as_deref(), Some("high"));
    }

    #[test]
    fn normalize_fills_reasoning_content_for_assistant_tool_call_when_thinking_enabled() {
        let mut request = request_with_messages(vec![assistant_tool_call_message(None)]);
        request.thinking = Some(ThinkingConfig {
            kind: ThinkingType::Enabled,
        });

        normalize_tokenhub_request(&mut request);

        assert_eq!(request.messages[0].reasoning_content.as_deref(), Some(" "));
    }

    #[test]
    fn normalize_does_not_fill_reasoning_content_when_thinking_disabled() {
        let mut request = request_with_messages(vec![assistant_tool_call_message(None)]);
        request.thinking = Some(ThinkingConfig {
            kind: ThinkingType::Disabled,
        });

        normalize_tokenhub_request(&mut request);

        assert_eq!(request.messages[0].reasoning_content, None);
    }

    #[test]
    fn normalize_keeps_existing_reasoning_content() {
        let mut request = request_with_messages(vec![assistant_tool_call_message(Some("trace"))]);
        request.thinking = Some(ThinkingConfig {
            kind: ThinkingType::Enabled,
        });

        normalize_tokenhub_request(&mut request);

        assert_eq!(
            request.messages[0].reasoning_content.as_deref(),
            Some("trace")
        );
    }

    #[test]
    fn normalize_fills_reasoning_content_for_empty_string() {
        let mut request = request_with_messages(vec![assistant_tool_call_message(Some(""))]);
        request.thinking = Some(ThinkingConfig {
            kind: ThinkingType::Enabled,
        });

        normalize_tokenhub_request(&mut request);

        assert_eq!(request.messages[0].reasoning_content.as_deref(), Some(" "));
    }

    #[test]
    fn normalize_fills_reasoning_content_with_default_thinking() {
        let mut request = request_with_messages(vec![assistant_tool_call_message(None)]);

        normalize_tokenhub_request(&mut request);

        assert_eq!(request.messages[0].reasoning_content.as_deref(), Some(" "));
    }

    #[test]
    fn normalize_fills_reasoning_content_with_adaptive_thinking() {
        let mut request = request_with_messages(vec![assistant_tool_call_message(None)]);
        request.thinking = Some(ThinkingConfig {
            kind: ThinkingType::Adaptive,
        });

        normalize_tokenhub_request(&mut request);

        assert_eq!(request.messages[0].reasoning_content.as_deref(), Some(" "));
    }

    #[test]
    fn map_reasoning_effort_to_tokenhub_values() {
        assert_eq!(map_reasoning_effort_to_tokenhub("minimal"), "low");
        assert_eq!(map_reasoning_effort_to_tokenhub("low"), "low");
        assert_eq!(map_reasoning_effort_to_tokenhub("medium"), "medium");
        assert_eq!(map_reasoning_effort_to_tokenhub("high"), "high");
        assert_eq!(map_reasoning_effort_to_tokenhub("max"), "high");
    }

    #[test]
    fn map_openai_error_keeps_upstream_status_for_non_400() {
        let err = ClientError::Api(ApiError::OpenAI {
            status: reqwest::StatusCode::PAYMENT_REQUIRED,
            error: crate::openai_types::ErrorDetail {
                message: "endpoint is inactive: FREE_QUOTA_EXHAUSTED".to_string(),
                r#type: "invalid_request_error".to_string(),
                code: Some("FREE_QUOTA_EXHAUSTED".to_string()),
                param: None,
            },
        });

        match map_openai_error(err) {
            ProviderError::Internal {
                upstream_http_status,
                message,
            } => {
                assert_eq!(upstream_http_status, StatusCode::PAYMENT_REQUIRED);
                assert!(message.contains("FREE_QUOTA_EXHAUSTED"));
            }
            other => panic!("expected internal error, got {other:?}"),
        }
    }

    #[test]
    fn parse_tokenhub_error_payload_returns_custom_value() {
        let body = br#"{
            "error": {
                "type": "invalid_request_error",
                "code": "bad_param",
                "message": "invalid request",
                "source": "upstream",
                "upstream_code": "E1001",
                "upstream_status": 400,
                "request_id": "req_123"
            }
        }"#;

        let parsed = parse_tokenhub_error_payload(body);
        assert!(parsed.is_some());
    }

    #[test]
    fn map_tokenhub_custom_error_maps_and_appends_metadata() {
        let value = serde_json::json!({
            "error": {
                "type": "invalid_request_error",
                "code": "bad_param",
                "message": "invalid request",
                "source": "upstream",
                "upstream_code": "E1001",
                "upstream_status": 400,
                "request_id": "req_123"
            }
        });

        let error = map_tokenhub_custom_error(value).expect("map tokenhub error");
        assert_eq!(error.r#type, "invalid_request_error");
        assert_eq!(error.code.as_deref(), Some("bad_param"));
        assert!(error.message.contains("invalid request"));
        assert!(error.message.contains("source=upstream"));
        assert!(error.message.contains("upstream_code=E1001"));
        assert!(error.message.contains("upstream_status=400"));
        assert!(error.message.contains("request_id=req_123"));
    }

    #[test]
    fn map_openai_error_maps_custom_tokenhub_error_to_public() {
        let err = ClientError::Api(ApiError::Custom {
            status: Some(StatusCode::UNPROCESSABLE_ENTITY),
            value: serde_json::json!({
                "error": {
                    "type": "invalid_request_error",
                    "code": "bad_param",
                    "message": "invalid request",
                    "source": "gateway",
                    "upstream_status": 422,
                    "request_id": "req_456"
                }
            }),
        });

        match map_openai_error(err) {
            ProviderError::Public { status, error } => {
                assert_eq!(status, StatusCode::UNPROCESSABLE_ENTITY);
                assert_eq!(error.r#type, "invalid_request_error");
                assert_eq!(error.code.as_deref(), Some("bad_param"));
                assert!(error.message.contains("request_id=req_456"));
            }
            other => panic!("expected public error, got {other:?}"),
        }
    }

    #[test]
    fn map_openai_error_maps_custom_tokenhub_error_without_status_to_public() {
        let err = ClientError::Api(ApiError::Custom {
            status: None,
            value: serde_json::json!({
                "error": {
                    "type": "invalid_request_error",
                    "code": "bad_param",
                    "message": "invalid request",
                    "source": "gateway",
                    "upstream_status": 422,
                    "request_id": "req_789"
                }
            }),
        });

        match map_openai_error(err) {
            ProviderError::Public { status, error } => {
                assert_eq!(status, StatusCode::BAD_GATEWAY);
                assert_eq!(error.r#type, "invalid_request_error");
                assert_eq!(error.code.as_deref(), Some("bad_param"));
                assert!(error.message.contains("request_id=req_789"));
            }
            other => panic!("expected public error, got {other:?}"),
        }
    }

    #[test]
    fn map_tokenhub_custom_error_keeps_empty_type_when_missing() {
        let value = serde_json::json!({
            "error": {
                "code": "bad_param",
                "message": "invalid request"
            }
        });

        let error = map_tokenhub_custom_error(value).expect("map tokenhub error");
        assert_eq!(error.r#type, "");
    }
}
