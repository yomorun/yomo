use async_stream::try_stream;
use async_trait::async_trait;
use axum::http::StatusCode;
use futures_core::Stream;
use futures_util::StreamExt;
use serde_json::Value;
use std::collections::HashMap;
use std::pin::Pin;

use crate::llm_provider::openai_compatible::client::{ApiError, ClientError};
use crate::llm_provider::openai_compatible::{client, mapper};
use crate::llm_provider::{Provider, ProviderError, UnifiedEvent, UnifiedResponse};
use crate::openai_http_mapping::validate_openai_request;
use crate::openai_types::{ChatCompletionRequest, Content, ContentPart, ErrorDetail, Role};
use crate::serve_config::ConfigError;

#[derive(Clone)]
pub struct VllmDeepseekProvider {
    client: client::Client,
    model_id: Option<String>,
}

impl VllmDeepseekProvider {
    pub fn new(client: client::Client, model_id: Option<String>) -> Self {
        Self { client, model_id }
    }
}

#[async_trait]
impl Provider for VllmDeepseekProvider {
    fn model_id(&self) -> &str {
        self.model_id.as_deref().unwrap_or("deepseek-v4-flash")
    }

    async fn complete(
        &self,
        mut request: ChatCompletionRequest,
    ) -> Result<UnifiedResponse, ProviderError> {
        if let Some(model_id) = &self.model_id {
            request.model = model_id.clone();
        }
        let request = normalize_request(request)?;
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
    ) -> Result<
        Pin<Box<dyn Stream<Item = Result<UnifiedEvent, ProviderError>> + Send + 'a>>,
        ProviderError,
    > {
        if let Some(model_id) = &self.model_id {
            request.model = model_id.clone();
        }
        let request = normalize_request(request)?;
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

pub fn build_vllm_deepseek_provider(
    params: &HashMap<String, String>,
) -> Result<VllmDeepseekProvider, ConfigError> {
    let api_key = params
        .get("api_key")
        .ok_or_else(|| ConfigError::InvalidProvider("api_key is required".to_string()))?;
    let mut config = client::Config::new(api_key.to_string());
    let model_id = params.get("model").cloned();
    if let Some(base_url) = params.get("base_url") {
        config = config.base_url(base_url.to_string());
    }
    let client =
        client::Client::new(config).map_err(|err| ConfigError::InvalidProvider(err.to_string()))?;
    Ok(VllmDeepseekProvider::new(client, model_id))
}

fn validate_request(request: &ChatCompletionRequest) -> Result<(), ProviderError> {
    validate_openai_request(request).map_err(invalid_request)
}

fn normalize_request(
    mut request: ChatCompletionRequest,
) -> Result<ChatCompletionRequest, ProviderError> {
    ensure_no_image_parts(&request)?;
    flatten_assistant_text_content(&mut request);
    normalize_reasoning_effort(&mut request);
    Ok(request)
}

fn ensure_no_image_parts(request: &ChatCompletionRequest) -> Result<(), ProviderError> {
    for message in &request.messages {
        if let Content::Parts(parts) = &message.content {
            for part in parts {
                if matches!(part, ContentPart::Image { .. }) {
                    return Err(invalid_request(
                        "vllm deepseek models do not support image_url messages",
                    ));
                }
            }
        }
    }
    Ok(())
}

fn flatten_assistant_text_content(request: &mut ChatCompletionRequest) {
    for message in &mut request.messages {
        if message.role != Role::Assistant {
            continue;
        }
        let mut merged_text: Option<String> = None;
        if let Content::Parts(parts) = &message.content {
            let mut combined = String::new();
            let mut all_text = true;
            for part in parts {
                if let ContentPart::Text { text } = part {
                    combined.push_str(text);
                } else {
                    all_text = false;
                    break;
                }
            }
            if all_text {
                merged_text = Some(combined);
            }
        }
        if let Some(text) = merged_text {
            message.content = Content::Text(text);
        }
    }
}

fn normalize_reasoning_effort(request: &mut ChatCompletionRequest) {
    let effort = request.reasoning_effort.take();
    let Some(effort) = effort else {
        return;
    };

    let has_thinking = request
        .chat_template_kwargs
        .as_ref()
        .map(|kwargs| kwargs.contains_key("thinking"))
        .unwrap_or(false);
    if has_thinking {
        return;
    }

    if matches!(effort.as_str(), "low" | "medium" | "high" | "max") {
        let mut kwargs = HashMap::new();
        kwargs.insert("thinking".to_string(), Value::Bool(true));
        kwargs.insert("reasoning_effort".to_string(), Value::String(effort));
        request.chat_template_kwargs = Some(kwargs);
    }
}

fn map_openai_error(err: ClientError) -> ProviderError {
    match err {
        ClientError::Api(ApiError::OpenAI { status, error }) => {
            if status.is_server_error() {
                upstream_service_error("http_service_error")
            } else {
                ProviderError::Public {
                    status: StatusCode::from_u16(status.as_u16())
                        .unwrap_or(StatusCode::INTERNAL_SERVER_ERROR),
                    error,
                }
            }
        }
        ClientError::Api(ApiError::Unknown { status, .. }) if status.is_server_error() => {
            upstream_service_error("http_service_error")
        }
        ClientError::Api(ApiError::Unknown { status, body }) => {
            ProviderError::internal_with_upstream_status(status, body)
        }
        ClientError::Timeout(_) => upstream_service_error("compute_resource_error"),
        ClientError::Http(error) if error.is_timeout() => {
            upstream_service_error("compute_resource_error")
        }
        ClientError::Http(_) => upstream_service_error("network_error"),
        other => ProviderError::internal(other.to_string()),
    }
}

fn upstream_service_error(code: &str) -> ProviderError {
    ProviderError::Public {
        status: StatusCode::INTERNAL_SERVER_ERROR,
        error: ErrorDetail {
            message: code.to_string(),
            r#type: "upstream_error".to_string(),
            code: Some(code.to_string()),
            param: None,
        },
    }
}

fn invalid_request(message: impl Into<String>) -> ProviderError {
    ProviderError::Public {
        status: StatusCode::BAD_REQUEST,
        error: ErrorDetail {
            message: message.into(),
            r#type: "invalid_request_error".to_string(),
            code: None,
            param: None,
        },
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::openai_types::{
        Content, ImageUrl, Message, ThinkingConfig, ThinkingType, ToolCall, ToolCallFunction,
    };
    use serde_json::json;

    fn request_with_messages(messages: Vec<Message>) -> ChatCompletionRequest {
        ChatCompletionRequest {
            model: "deepseek".to_string(),
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

    fn user_text_message(text: &str) -> Message {
        Message {
            role: Role::User,
            content: Content::Text(text.to_string()),
            reasoning_content: None,
            tool_call_id: None,
            tool_calls: None,
        }
    }

    #[test]
    fn normalize_reasoning_effort_sets_chat_template_kwargs() {
        let mut request = request_with_messages(vec![user_text_message("hello")]);
        request.reasoning_effort = Some("high".to_string());

        normalize_reasoning_effort(&mut request);

        let kwargs = request.chat_template_kwargs.expect("chat_template_kwargs");
        assert_eq!(kwargs.get("thinking"), Some(&Value::Bool(true)));
        assert_eq!(
            kwargs.get("reasoning_effort"),
            Some(&Value::String("high".to_string()))
        );
    }

    #[test]
    fn normalize_reasoning_effort_keeps_existing_thinking_kwargs() {
        let mut request = request_with_messages(vec![user_text_message("hello")]);
        request.reasoning_effort = Some("medium".to_string());
        request.chat_template_kwargs = Some(HashMap::from([(
            "thinking".to_string(),
            Value::Bool(false),
        )]));

        normalize_reasoning_effort(&mut request);

        let kwargs = request.chat_template_kwargs.expect("chat_template_kwargs");
        assert_eq!(kwargs.get("thinking"), Some(&Value::Bool(false)));
        assert_eq!(kwargs.get("reasoning_effort"), None);
    }

    #[test]
    fn flatten_assistant_text_content_merges_text_parts() {
        let assistant = Message {
            role: Role::Assistant,
            content: Content::Parts(vec![
                crate::openai_types::ContentPart::Text {
                    text: "hello ".to_string(),
                },
                crate::openai_types::ContentPart::Text {
                    text: "world".to_string(),
                },
            ]),
            reasoning_content: None,
            tool_call_id: None,
            tool_calls: Some(vec![ToolCall {
                id: Some("call_1".to_string()),
                r#type: Some("function".to_string()),
                function: ToolCallFunction {
                    name: "noop".to_string(),
                    arguments: "{}".to_string(),
                    description: None,
                },
            }]),
        };
        let mut request = request_with_messages(vec![assistant]);

        flatten_assistant_text_content(&mut request);

        match &request.messages[0].content {
            Content::Text(text) => assert_eq!(text, "hello world"),
            _ => panic!("assistant content should be flattened to text"),
        }
    }

    #[test]
    fn ensure_no_image_parts_returns_error() {
        let request = request_with_messages(vec![Message {
            role: Role::User,
            content: Content::Parts(vec![crate::openai_types::ContentPart::Image {
                image_url: ImageUrl {
                    url: "https://example.com/a.png".to_string(),
                    detail: None,
                },
            }]),
            reasoning_content: None,
            tool_call_id: None,
            tool_calls: None,
        }]);

        let err = ensure_no_image_parts(&request).expect_err("should reject image parts");
        match err {
            ProviderError::Public { status, error } => {
                assert_eq!(status, axum::http::StatusCode::BAD_REQUEST);
                assert_eq!(error.r#type, "invalid_request_error");
                assert!(
                    error
                        .message
                        .contains("vllm deepseek models do not support image_url messages")
                );
            }
            other => panic!("expected public bad request, got {other:?}"),
        }
    }

    #[test]
    fn validate_request_allows_empty_content() {
        let request = request_with_messages(vec![user_text_message("   ")]);

        let result = validate_request(&request);
        assert!(result.is_ok());
    }

    #[test]
    fn map_openai_error_openai_api_error_becomes_public() {
        let err = ClientError::Api(ApiError::OpenAI {
            status: reqwest::StatusCode::TOO_MANY_REQUESTS,
            error: ErrorDetail {
                message: "rate limit exceeded".to_string(),
                r#type: "rate_limit_error".to_string(),
                code: Some("rate_limit".to_string()),
                param: None,
            },
        });

        match map_openai_error(err) {
            ProviderError::Public { status, error } => {
                assert_eq!(status, axum::http::StatusCode::TOO_MANY_REQUESTS);
                assert_eq!(error.message, "rate limit exceeded");
                assert_eq!(error.r#type, "rate_limit_error");
                assert_eq!(error.code.as_deref(), Some("rate_limit"));
            }
            other => panic!("expected public error, got {other:?}"),
        }
    }

    #[test]
    fn map_openai_error_openai_5xx_becomes_http_service_error() {
        let err = ClientError::Api(ApiError::OpenAI {
            status: reqwest::StatusCode::INTERNAL_SERVER_ERROR,
            error: ErrorDetail {
                message: "upstream failed".to_string(),
                r#type: "server_error".to_string(),
                code: None,
                param: None,
            },
        });

        match map_openai_error(err) {
            ProviderError::Public { status, error } => {
                assert_eq!(status, axum::http::StatusCode::INTERNAL_SERVER_ERROR);
                assert_eq!(error.message, "http_service_error");
                assert_eq!(error.r#type, "upstream_error");
                assert_eq!(error.code.as_deref(), Some("http_service_error"));
            }
            other => panic!("expected public error, got {other:?}"),
        }
    }

    #[test]
    fn map_openai_error_timeout_becomes_compute_resource_error() {
        let err = ClientError::Timeout("stream first byte timeout".to_string());

        match map_openai_error(err) {
            ProviderError::Public { status, error } => {
                assert_eq!(status, axum::http::StatusCode::INTERNAL_SERVER_ERROR);
                assert_eq!(error.message, "compute_resource_error");
                assert_eq!(error.r#type, "upstream_error");
                assert_eq!(error.code.as_deref(), Some("compute_resource_error"));
            }
            other => panic!("expected public error, got {other:?}"),
        }
    }

    #[test]
    fn map_openai_error_unknown_5xx_becomes_http_service_error() {
        let err = ClientError::Api(ApiError::Unknown {
            status: reqwest::StatusCode::BAD_GATEWAY,
            body: "gateway failed".to_string(),
        });

        match map_openai_error(err) {
            ProviderError::Public { status, error } => {
                assert_eq!(status, axum::http::StatusCode::INTERNAL_SERVER_ERROR);
                assert_eq!(error.message, "http_service_error");
                assert_eq!(error.r#type, "upstream_error");
                assert_eq!(error.code.as_deref(), Some("http_service_error"));
            }
            other => panic!("expected public error, got {other:?}"),
        }
    }

    #[test]
    fn normalize_request_combines_compatibility_steps() {
        let assistant = Message {
            role: Role::Assistant,
            content: Content::Parts(vec![crate::openai_types::ContentPart::Text {
                text: "ok".to_string(),
            }]),
            reasoning_content: None,
            tool_call_id: None,
            tool_calls: None,
        };
        let mut request = request_with_messages(vec![assistant]);
        request.reasoning_effort = Some("max".to_string());
        request.thinking = Some(ThinkingConfig {
            kind: ThinkingType::Disabled,
        });
        request.chat_template_kwargs = Some(HashMap::from([(
            "temperature_hint".to_string(),
            json!("ignored by normalize_reasoning_effort overwrite"),
        )]));

        let normalized = normalize_request(request).expect("normalize request");

        assert!(matches!(&normalized.messages[0].content, Content::Text(text) if text == "ok"));
        let kwargs = normalized
            .chat_template_kwargs
            .expect("chat_template_kwargs set by reasoning effort");
        assert_eq!(kwargs.get("thinking"), Some(&Value::Bool(true)));
        assert_eq!(
            kwargs.get("reasoning_effort"),
            Some(&Value::String("max".to_string()))
        );
    }
}
