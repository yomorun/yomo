use std::collections::HashMap;
use std::pin::Pin;
use std::sync::Arc;
use std::sync::atomic::{AtomicI32, Ordering};

use async_stream::try_stream;
use async_trait::async_trait;
use axum::http::StatusCode;
use futures_core::Stream;
use futures_util::StreamExt;
use log::{info, warn};

use crate::llm_provider::{Provider, ProviderError, UnifiedEvent, UnifiedResponse};
use crate::openai_http_mapping::validate_openai_request;
use crate::openai_types::ChatCompletionRequest;
use crate::serve_config::ConfigError;

use self::client::{
    Backend, BedrockClient, ClientError, DirectClient, map_client_error, parse_auth_style,
};
use self::mapper::{
    inline_image_urls, map_request, map_response, map_stop_reason_string, map_stream_event,
};
use self::types::{
    AnthropicRequest, BedrockRequest, DEFAULT_ANTHROPIC_VERSION, DEFAULT_BEDROCK_ANTHROPIC_VERSION,
    StreamState,
};

mod client;
mod mapper;
mod types;

/// Anthropic requires `max_tokens`. When neither the client nor the config sets one and the
/// model's limit is not known yet, send this value: the upstream rejects it with the model's
/// maximum output tokens, which is cached and used for the retry and later requests.
const PROBE_MAX_TOKENS: i32 = i32::MAX;

#[derive(Clone)]
pub struct AnthropicMessagesProvider {
    backend: Backend,
    upstream_model: String,
    anthropic_version: String,
    configured_max_tokens: Option<i32>,
    /// The model's maximum output tokens learned from the upstream; 0 until known.
    model_max_tokens: Arc<AtomicI32>,
    image_client: reqwest::Client,
}

impl AnthropicMessagesProvider {
    fn new(
        backend: Backend,
        upstream_model: String,
        anthropic_version: String,
        configured_max_tokens: Option<i32>,
    ) -> Self {
        Self {
            backend,
            upstream_model,
            anthropic_version,
            configured_max_tokens,
            model_max_tokens: Arc::new(AtomicI32::new(0)),
            image_client: reqwest::Client::new(),
        }
    }

    /// Output limit for requests without `max_completion_tokens`: the configured `max_tokens`,
    /// else the model's learned maximum.
    fn default_max_tokens(&self) -> Option<i32> {
        self.configured_max_tokens.or_else(|| {
            let learned = self.model_max_tokens.load(Ordering::Relaxed);
            (learned > 0).then_some(learned)
        })
    }

    /// If the upstream rejected [`PROBE_MAX_TOKENS`], cache the model's maximum from the error
    /// and return it for a retry.
    fn learn_model_max_tokens<T>(
        &self,
        probing: bool,
        result: &Result<T, ClientError>,
    ) -> Option<i32> {
        if !probing {
            return None;
        }
        let Err(ClientError::Api { status, error }) = result else {
            return None;
        };
        if *status != StatusCode::BAD_REQUEST {
            return None;
        }
        let limit = parse_max_output_tokens(&error.message)?;
        self.model_max_tokens.store(limit, Ordering::Relaxed);
        info!(
            "anthropic messages model max_tokens learned; model={} max_tokens={}",
            self.upstream_model, limit
        );
        Some(limit)
    }
}

/// Parse the limit from "max_tokens: 2147483647 > 128000, which is the maximum allowed number
/// of output tokens for ...".
fn parse_max_output_tokens(message: &str) -> Option<i32> {
    let (_, rest) = message.split_once(" > ")?;
    let (limit, tail) = rest.split_once(',')?;
    if !tail.contains("maximum allowed number of output tokens") {
        return None;
    }
    limit.trim().parse().ok().filter(|limit| *limit > 0)
}

#[async_trait]
impl<M> Provider<M> for AnthropicMessagesProvider {
    fn model_id(&self) -> &str {
        "anthropic-messages"
    }

    async fn complete(
        &self,
        request: ChatCompletionRequest,
        _metadata: &M,
    ) -> Result<UnifiedResponse, ProviderError> {
        validate_openai_request(&request).map_err(ProviderError::internal)?;
        let mut request = request;
        inline_image_urls(&mut request, &self.image_client).await?;
        let default_max_tokens = self.default_max_tokens();
        let probing = request.max_completion_tokens.is_none() && default_max_tokens.is_none();
        let mapped = map_request(
            request,
            self.upstream_model.clone(),
            default_max_tokens.unwrap_or(PROBE_MAX_TOKENS),
            true,
        )?;
        let response = match &self.backend {
            Backend::Direct(client) => {
                let mut payload = AnthropicRequest {
                    model: mapped.model,
                    max_tokens: mapped.max_tokens,
                    messages: mapped.messages,
                    system: mapped.system,
                    temperature: mapped.temperature,
                    top_p: mapped.top_p,
                    stop_sequences: mapped.stop_sequences,
                    stream: Some(false),
                    tools: mapped.tools,
                    tool_choice: mapped.tool_choice,
                    thinking: mapped.thinking,
                    output_config: mapped.output_config,
                };
                let mut result = client
                    .send_complete(&payload, &self.anthropic_version)
                    .await;
                if let Some(limit) = self.learn_model_max_tokens(probing, &result) {
                    payload.max_tokens = limit;
                    result = client
                        .send_complete(&payload, &self.anthropic_version)
                        .await;
                }
                result.map_err(map_client_error)?
            }
            Backend::Bedrock(client) => {
                let mut payload = BedrockRequest {
                    anthropic_version: self.anthropic_version.clone(),
                    max_tokens: mapped.max_tokens,
                    messages: mapped.messages,
                    system: mapped.system,
                    temperature: mapped.temperature,
                    top_p: mapped.top_p,
                    stop_sequences: mapped.stop_sequences,
                    tools: mapped.tools,
                    tool_choice: mapped.tool_choice,
                    thinking: mapped.thinking,
                    output_config: mapped.output_config,
                };
                let mut result = client.send_complete(&payload).await;
                if let Some(limit) = self.learn_model_max_tokens(probing, &result) {
                    payload.max_tokens = limit;
                    result = client.send_complete(&payload).await;
                }
                result.map_err(map_client_error)?
            }
        };

        Ok(map_response(response))
    }

    async fn stream<'a>(
        &'a self,
        request: ChatCompletionRequest,
        _metadata: &M,
    ) -> Result<
        Pin<Box<dyn Stream<Item = Result<UnifiedEvent, ProviderError>> + Send + 'a>>,
        ProviderError,
    > {
        validate_openai_request(&request).map_err(ProviderError::internal)?;
        let mut request = request;
        inline_image_urls(&mut request, &self.image_client).await?;
        let default_max_tokens = self.default_max_tokens();
        let probing = request.max_completion_tokens.is_none() && default_max_tokens.is_none();
        let mapped = map_request(
            request,
            self.upstream_model.clone(),
            default_max_tokens.unwrap_or(PROBE_MAX_TOKENS),
            true,
        )?;

        let stream = match &self.backend {
            Backend::Direct(client) => {
                let mut payload = AnthropicRequest {
                    model: mapped.model,
                    max_tokens: mapped.max_tokens,
                    messages: mapped.messages,
                    system: mapped.system,
                    temperature: mapped.temperature,
                    top_p: mapped.top_p,
                    stop_sequences: mapped.stop_sequences,
                    stream: Some(true),
                    tools: mapped.tools,
                    tool_choice: mapped.tool_choice,
                    thinking: mapped.thinking,
                    output_config: mapped.output_config,
                };
                let mut result = client.send_stream(&payload, &self.anthropic_version).await;
                if let Some(limit) = self.learn_model_max_tokens(probing, &result) {
                    payload.max_tokens = limit;
                    result = client.send_stream(&payload, &self.anthropic_version).await;
                }
                result.map_err(map_client_error)?
            }
            Backend::Bedrock(client) => {
                let mut payload = BedrockRequest {
                    anthropic_version: self.anthropic_version.clone(),
                    max_tokens: mapped.max_tokens,
                    messages: mapped.messages,
                    system: mapped.system,
                    temperature: mapped.temperature,
                    top_p: mapped.top_p,
                    stop_sequences: mapped.stop_sequences,
                    tools: mapped.tools,
                    tool_choice: mapped.tool_choice,
                    thinking: mapped.thinking,
                    output_config: mapped.output_config,
                };
                let mut result = client.send_stream(&payload).await;
                if let Some(limit) = self.learn_model_max_tokens(probing, &result) {
                    payload.max_tokens = limit;
                    result = client.send_stream(&payload).await;
                }
                result.map_err(map_client_error)?
            }
        };

        let output = try_stream! {
            futures_util::pin_mut!(stream);
            let mut state = StreamState::default();
            while let Some(item) = stream.next().await {
                let event = item?;
                for mapped_event in map_stream_event(event, &mut state) {
                    yield mapped_event;
                }
            }

            if !state.usage_received {
                warn!(
                    "anthropic messages upstream stream missing usage; model={} request_id={}",
                    state.model, state.response_id
                );
            }

            if state.started && !state.completed {
                let stop_reason = state
                    .stop_reason
                    .clone()
                    .unwrap_or_else(|| "end_turn".to_string());
                yield UnifiedEvent::MessageStop {
                    id: state.response_id,
                    stop_reason: Some(map_stop_reason_string(&stop_reason).to_string()),
                };
                yield UnifiedEvent::Completed {
                    finish_reason: Some(map_stop_reason_string(&stop_reason).to_string()),
                };
            }
        };

        Ok(Box::pin(output))
    }
}

pub fn build_anthropic_messages_provider(
    params: &HashMap<String, String>,
) -> Result<AnthropicMessagesProvider, ConfigError> {
    let api_key = params
        .get("api_key")
        .cloned()
        .ok_or_else(|| ConfigError::InvalidProvider("api_key is required".to_string()))?;
    let base_url = params
        .get("base_url")
        .cloned()
        .ok_or_else(|| ConfigError::InvalidProvider("base_url is required".to_string()))?;
    let upstream_model = params
        .get("model")
        .cloned()
        .ok_or_else(|| ConfigError::InvalidProvider("model is required".to_string()))?;
    let anthropic_version = params
        .get("anthropic_version")
        .cloned()
        .unwrap_or_else(|| DEFAULT_ANTHROPIC_VERSION.to_string());
    let auth_style = parse_auth_style(params.get("auth_style"))?;

    Ok(AnthropicMessagesProvider::new(
        Backend::Direct(DirectClient {
            client: reqwest::Client::new(),
            base_url,
            auth_style,
            api_key,
        }),
        upstream_model,
        anthropic_version,
        parse_max_tokens(params),
    ))
}

pub fn build_bedrock_messages_provider(
    params: &HashMap<String, String>,
) -> Result<AnthropicMessagesProvider, ConfigError> {
    let bedrock_model = params
        .get("model")
        .cloned()
        .ok_or_else(|| ConfigError::InvalidProvider("model is required".to_string()))?;
    let aws_region = params
        .get("aws_region")
        .cloned()
        .ok_or_else(|| ConfigError::InvalidProvider("aws_region is required".to_string()))?;
    let aws_bearer_token = params
        .get("aws_bearer_token")
        .map(|token| token.trim().to_string())
        .filter(|token| !token.is_empty())
        .ok_or_else(|| ConfigError::InvalidProvider("aws_bearer_token is required".to_string()))?;
    let anthropic_version = params
        .get("anthropic_version")
        .cloned()
        .unwrap_or_else(|| DEFAULT_BEDROCK_ANTHROPIC_VERSION.to_string());
    Ok(AnthropicMessagesProvider::new(
        Backend::Bedrock(BedrockClient {
            model_id: bedrock_model.clone(),
            aws_region,
            aws_bearer_token,
        }),
        bedrock_model,
        anthropic_version,
        parse_max_tokens(params),
    ))
}

/// Optional fixed `max_tokens` for requests that do not set `max_completion_tokens`.
fn parse_max_tokens(params: &HashMap<String, String>) -> Option<i32> {
    params
        .get("max_tokens")
        .and_then(|raw| raw.parse::<i32>().ok())
        .filter(|max_tokens| *max_tokens > 0)
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;
    use crate::llm_provider::anthropic_messages::types::AnthropicContentBlock;
    use crate::llm_provider::anthropic_messages::types::{
        StreamContentBlock, StreamContentDelta, StreamEvent, StreamMessage,
    };
    use crate::openai_types::{
        Content, FunctionDefinition, Message, Role, ToolChoice, ToolDefinition,
    };

    fn basic_chat_request(temperature: Option<f32>, top_p: Option<f32>) -> ChatCompletionRequest {
        ChatCompletionRequest {
            model: "alias".to_string(),
            messages: vec![Message {
                role: Role::User,
                content: Content::Text("hello".to_string()),
                reasoning_content: None,
                tool_call_id: None,
                tool_calls: None,
            }],
            n: None,
            temperature,
            top_p,
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

    #[test]
    fn build_anthropic_messages_provider_accepts_x_api_key_auth_style() {
        let mut params = HashMap::new();
        params.insert("api_key".to_string(), "sk-ant-test".to_string());
        params.insert(
            "base_url".to_string(),
            "https://api.anthropic.com/v1".to_string(),
        );
        params.insert("model".to_string(), "claude-sonnet-5".to_string());

        let provider = build_anthropic_messages_provider(&params)
            .expect("anthropic messages provider should build");

        assert_eq!(provider.upstream_model, "claude-sonnet-5");
    }

    #[test]
    fn build_anthropic_messages_provider_accepts_bearer_auth_style() {
        let mut params = HashMap::new();
        params.insert("api_key".to_string(), "token".to_string());
        params.insert(
            "base_url".to_string(),
            "https://proxy.example.com/v1".to_string(),
        );
        params.insert("model".to_string(), "claude-sonnet-5".to_string());
        params.insert("auth_style".to_string(), "bearer".to_string());

        let provider = build_anthropic_messages_provider(&params)
            .expect("anthropic messages provider should build");

        assert_eq!(provider.anthropic_version, DEFAULT_ANTHROPIC_VERSION);
    }

    #[test]
    fn build_bedrock_messages_provider_accepts_required_params() {
        let mut params = HashMap::new();
        params.insert(
            "model".to_string(),
            "global.anthropic.claude-sonnet-4-6".to_string(),
        );
        params.insert("aws_region".to_string(), "ap-northeast-1".to_string());
        params.insert("aws_bearer_token".to_string(), "token".to_string());

        let provider = build_bedrock_messages_provider(&params)
            .expect("bedrock messages provider should build");

        assert_eq!(provider.configured_max_tokens, None);
    }

    fn bedrock_provider(max_tokens: Option<&str>) -> AnthropicMessagesProvider {
        let mut params = HashMap::new();
        params.insert(
            "model".to_string(),
            "global.anthropic.claude-haiku-5-5".to_string(),
        );
        params.insert("aws_region".to_string(), "eu-central-1".to_string());
        params.insert("aws_bearer_token".to_string(), "token".to_string());
        if let Some(max_tokens) = max_tokens {
            params.insert("max_tokens".to_string(), max_tokens.to_string());
        }
        build_bedrock_messages_provider(&params).expect("bedrock messages provider should build")
    }

    fn api_error(status: StatusCode, message: &str) -> Result<(), ClientError> {
        Err(ClientError::Api {
            status,
            error: crate::openai_types::ErrorDetail {
                message: message.to_string(),
                r#type: "invalid_request_error".to_string(),
                code: None,
                param: None,
            },
        })
    }

    const MAX_TOKENS_LIMIT_MESSAGE: &str = "max_tokens: 2147483647 > 128000, which is the maximum allowed number of output tokens for anthropic.claude-haiku-5-5";

    #[test]
    fn configured_max_tokens_is_the_default() {
        let provider = bedrock_provider(Some("32000"));

        assert_eq!(provider.default_max_tokens(), Some(32000));
    }

    #[test]
    fn learn_model_max_tokens_caches_upstream_limit() {
        let provider = bedrock_provider(None);
        assert_eq!(provider.default_max_tokens(), None);

        let learned = provider.learn_model_max_tokens(
            true,
            &api_error(StatusCode::BAD_REQUEST, MAX_TOKENS_LIMIT_MESSAGE),
        );

        assert_eq!(learned, Some(128000));
        assert_eq!(provider.clone().default_max_tokens(), Some(128000));
    }

    #[test]
    fn learn_model_max_tokens_ignores_other_errors() {
        let provider = bedrock_provider(None);

        let not_probing = provider.learn_model_max_tokens(
            false,
            &api_error(StatusCode::BAD_REQUEST, MAX_TOKENS_LIMIT_MESSAGE),
        );
        let other_message = provider.learn_model_max_tokens(
            true,
            &api_error(StatusCode::BAD_REQUEST, "messages: Field required"),
        );
        let other_status = provider.learn_model_max_tokens(
            true,
            &api_error(StatusCode::TOO_MANY_REQUESTS, MAX_TOKENS_LIMIT_MESSAGE),
        );

        assert_eq!(
            (not_probing, other_message, other_status),
            (None, None, None)
        );
        assert_eq!(provider.default_max_tokens(), None);
    }

    #[test]
    fn parse_max_output_tokens_reads_limit() {
        assert_eq!(
            parse_max_output_tokens(MAX_TOKENS_LIMIT_MESSAGE),
            Some(128000)
        );
        assert_eq!(
            parse_max_output_tokens("max_tokens: 5 > 3, which is not about output"),
            None
        );
    }

    #[test]
    fn map_request_maps_tool_messages_to_tool_result_blocks() {
        let request = ChatCompletionRequest {
            model: "alias".to_string(),
            messages: vec![
                Message {
                    role: Role::Assistant,
                    content: Content::Text(String::new()),
                    reasoning_content: None,
                    tool_call_id: None,
                    tool_calls: Some(vec![crate::openai_types::ToolCall {
                        id: Some("call_1".to_string()),
                        r#type: Some("function".to_string()),
                        function: crate::openai_types::ToolCallFunction {
                            name: "lookup".to_string(),
                            arguments: "{\"q\":\"x\"}".to_string(),
                            description: None,
                        },
                    }]),
                },
                Message {
                    role: Role::Tool,
                    content: Content::Text("ok".to_string()),
                    reasoning_content: None,
                    tool_call_id: Some("call_1".to_string()),
                    tool_calls: None,
                },
            ],
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
            tools: Some(vec![ToolDefinition {
                r#type: "function".to_string(),
                function: FunctionDefinition {
                    name: "lookup".to_string(),
                    description: None,
                    strict: None,
                    parameters: json!({"type": "object"}),
                },
            }]),
            tool_choice: Some(ToolChoice::Name("auto".to_string())),
            allowed_tools: None,
            parallel_tool_calls: Some(false),
            service_tier: None,
            seed: None,
            stream: None,
            stream_options: None,
            metadata: None,
            agent_context: None,
        };

        let mapped = map_request(request, "claude-sonnet-5".to_string(), 4096, false)
            .expect("request should map");

        assert_eq!(mapped.messages.len(), 2);
        assert!(matches!(
            mapped.messages[1].content[0],
            AnthropicContentBlock::ToolResult { .. }
        ));
    }

    #[test]
    fn map_request_skips_empty_assistant_message_without_tool_calls() {
        let request = ChatCompletionRequest {
            model: "alias".to_string(),
            messages: vec![
                Message {
                    role: Role::User,
                    content: Content::Text("hi".to_string()),
                    reasoning_content: None,
                    tool_call_id: None,
                    tool_calls: None,
                },
                Message {
                    role: Role::Assistant,
                    content: Content::Text(String::new()),
                    reasoning_content: None,
                    tool_call_id: None,
                    tool_calls: None,
                },
            ],
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
        };

        let mapped = map_request(request, "claude-sonnet-5".to_string(), 4096, false)
            .expect("request should map");

        assert_eq!(mapped.messages.len(), 1);
        assert!(matches!(
            mapped.messages[0].content[0],
            AnthropicContentBlock::Text { .. }
        ));
    }

    #[test]
    fn map_request_omits_empty_tool_result_text_content() {
        let request = ChatCompletionRequest {
            model: "alias".to_string(),
            messages: vec![Message {
                role: Role::Tool,
                content: Content::Text(String::new()),
                reasoning_content: None,
                tool_call_id: Some("call_1".to_string()),
                tool_calls: None,
            }],
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
        };

        let mapped = map_request(request, "claude-sonnet-5".to_string(), 4096, false)
            .expect("request should map");

        assert_eq!(mapped.messages.len(), 1);
        assert!(matches!(
            &mapped.messages[0].content[0],
            AnthropicContentBlock::ToolResult { content: None, .. }
        ));
    }

    #[test]
    fn map_request_disables_thinking_for_haiku_models() {
        let request = ChatCompletionRequest {
            model: "alias".to_string(),
            messages: vec![Message {
                role: Role::User,
                content: Content::Text("hello".to_string()),
                reasoning_content: None,
                tool_call_id: None,
                tool_calls: None,
            }],
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
            reasoning_effort: Some("high".to_string()),
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
        };

        let mapped = map_request(request, "claude-haiku-4-5".to_string(), 4096, false)
            .expect("request should map");

        assert!(mapped.thinking.is_none());
    }

    #[test]
    fn map_request_disables_thinking_for_prefixed_haiku_models() {
        let request = ChatCompletionRequest {
            model: "alias".to_string(),
            messages: vec![Message {
                role: Role::User,
                content: Content::Text("hello".to_string()),
                reasoning_content: None,
                tool_call_id: None,
                tool_calls: None,
            }],
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
            thinking: Some(crate::openai_types::ThinkingConfig {
                kind: crate::openai_types::ThinkingType::Enabled,
            }),
            reasoning_effort: Some("high".to_string()),
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
        };

        let mapped = map_request(
            request,
            "anthropic.claude-haiku-4-5".to_string(),
            4096,
            false,
        )
        .expect("request should map");

        assert!(mapped.thinking.is_none());
    }

    #[test]
    fn map_request_maps_reasoning_effort_to_output_config_effort() {
        let request = ChatCompletionRequest {
            model: "alias".to_string(),
            messages: vec![Message {
                role: Role::User,
                content: Content::Text("hello".to_string()),
                reasoning_content: None,
                tool_call_id: None,
                tool_calls: None,
            }],
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
            reasoning_effort: Some("high".to_string()),
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
        };

        let mapped = map_request(
            request,
            "global.anthropic.claude-sonnet-4-6".to_string(),
            4096,
            false,
        )
        .expect("request should map");

        assert!(mapped.thinking.is_none());
        assert_eq!(
            mapped.output_config.map(|config| config.effort).as_deref(),
            Some("high")
        );
    }

    #[test]
    fn map_request_keeps_thinking_for_haiku_5_5() {
        let mut request = basic_chat_request(None, None);
        request.thinking = Some(crate::openai_types::ThinkingConfig {
            kind: crate::openai_types::ThinkingType::Adaptive,
        });

        let mapped = map_request(
            request,
            "global.anthropic.claude-haiku-5-5".to_string(),
            4096,
            false,
        )
        .expect("request should map");

        assert!(matches!(
            mapped.thinking,
            Some(crate::llm_provider::anthropic_messages::types::AnthropicThinking::Adaptive)
        ));
    }

    #[test]
    fn map_request_maps_openai_only_effort_levels_to_low() {
        for effort in ["none", "minimal"] {
            let mut request = basic_chat_request(None, None);
            request.reasoning_effort = Some(effort.to_string());

            let mapped = map_request(
                request,
                "global.anthropic.claude-opus-5-5".to_string(),
                4096,
                false,
            )
            .expect("request should map");

            assert_eq!(
                mapped.output_config.map(|config| config.effort).as_deref(),
                Some("low")
            );
        }
    }

    #[test]
    fn map_request_omits_output_config_without_reasoning_effort() {
        let mapped = map_request(
            basic_chat_request(None, None),
            "global.anthropic.claude-opus-5-5".to_string(),
            4096,
            false,
        )
        .expect("request should map");

        assert!(mapped.output_config.is_none());
    }

    #[test]
    fn map_request_drops_non_default_sampling_for_restricted_models_when_compat_enabled() {
        let mut request = basic_chat_request(Some(0.2), Some(0.7));
        request.model = "claude-sonnet-5".to_string();

        let mapped = map_request(request, "claude-sonnet-5".to_string(), 4096, true)
            .expect("request should map");

        assert_eq!(mapped.temperature, None);
        assert_eq!(mapped.top_p, None);
    }

    #[test]
    fn map_request_keeps_non_default_sampling_for_non_restricted_models_when_compat_enabled() {
        let request = basic_chat_request(Some(0.2), Some(0.7));

        let mapped = map_request(request, "claude-sonnet-4-6".to_string(), 4096, true)
            .expect("request should map");

        assert_eq!(mapped.temperature, Some(0.2));
        assert_eq!(mapped.top_p, Some(0.7));
    }

    #[test]
    fn map_request_sampling_compat_checks_request_model_only() {
        let mut request = basic_chat_request(Some(0.2), Some(0.7));
        request.model = "global.anthropic.claude-sonnet-5".to_string();

        let mapped = map_request(request, "claude-sonnet-5".to_string(), 4096, true)
            .expect("request should map");

        assert_eq!(mapped.temperature, Some(0.2));
        assert_eq!(mapped.top_p, Some(0.7));
    }

    #[test]
    fn parse_auth_style_rejects_unknown() {
        let err = super::client::parse_auth_style(Some(&"unknown".to_string()))
            .err()
            .expect("unknown auth style should error");

        assert_eq!(
            err.to_string(),
            "invalid provider: unknown auth_style: unknown"
        );
    }

    #[test]
    fn map_stream_event_ignores_empty_input_json_delta() {
        let mut state = StreamState::default();

        let events = map_stream_event(
            StreamEvent::MessageStart {
                message: StreamMessage {
                    id: "msg_1".to_string(),
                    model: "claude-haiku-4-5".to_string(),
                },
            },
            &mut state,
        );
        assert!(!events.is_empty());

        let events = map_stream_event(
            StreamEvent::ContentBlockStart {
                index: 0,
                content_block: StreamContentBlock::ToolUse {
                    id: "toolu_1".to_string(),
                    name: "TaskList".to_string(),
                    input: json!({}),
                },
            },
            &mut state,
        );
        assert!(events.is_empty());

        let events = map_stream_event(
            StreamEvent::ContentBlockDelta {
                index: 0,
                delta: StreamContentDelta::InputJsonDelta {
                    partial_json: String::new(),
                },
            },
            &mut state,
        );
        assert!(events.is_empty());

        let events = map_stream_event(StreamEvent::ContentBlockStop { index: 0 }, &mut state);
        assert_eq!(events.len(), 1);
        assert!(matches!(
            &events[0],
            UnifiedEvent::ToolCallDone { arguments, .. } if arguments == "{}"
        ));
    }

    #[test]
    fn map_request_generates_unique_fallback_tool_call_ids() {
        let request = ChatCompletionRequest {
            model: "alias".to_string(),
            messages: vec![Message {
                role: Role::Assistant,
                content: Content::Text(String::new()),
                reasoning_content: None,
                tool_call_id: None,
                tool_calls: Some(vec![
                    crate::openai_types::ToolCall {
                        id: None,
                        r#type: Some("function".to_string()),
                        function: crate::openai_types::ToolCallFunction {
                            name: "lookup".to_string(),
                            arguments: "{\"q\":\"x\"}".to_string(),
                            description: None,
                        },
                    },
                    crate::openai_types::ToolCall {
                        id: None,
                        r#type: Some("function".to_string()),
                        function: crate::openai_types::ToolCallFunction {
                            name: "lookup".to_string(),
                            arguments: "{\"q\":\"y\"}".to_string(),
                            description: None,
                        },
                    },
                ]),
            }],
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
        };

        let mapped = map_request(request, "claude-sonnet-5".to_string(), 4096, false)
            .expect("request should map");

        let AnthropicContentBlock::ToolUse { id: first_id, .. } = &mapped.messages[0].content[0]
        else {
            panic!("expected first tool use block");
        };
        let AnthropicContentBlock::ToolUse { id: second_id, .. } = &mapped.messages[0].content[1]
        else {
            panic!("expected second tool use block");
        };

        assert_ne!(first_id, second_id);
    }

    #[test]
    fn map_stream_event_ignores_duplicate_message_stop() {
        let mut state = StreamState::default();

        let events = map_stream_event(
            StreamEvent::MessageStart {
                message: StreamMessage {
                    id: "msg_1".to_string(),
                    model: "claude-haiku-4-5".to_string(),
                },
            },
            &mut state,
        );
        assert!(!events.is_empty());

        let first_stop_events = map_stream_event(StreamEvent::MessageStop, &mut state);
        assert_eq!(first_stop_events.len(), 2);
        assert!(state.completed);

        let second_stop_events = map_stream_event(StreamEvent::MessageStop, &mut state);
        assert!(second_stop_events.is_empty());
    }

    #[test]
    fn map_stream_event_tracks_received_usage() {
        let mut state = StreamState::default();

        map_stream_event(
            StreamEvent::MessageDelta {
                delta: None,
                usage: Some(crate::model_api_provider::MessagesUsage {
                    input_tokens: Some(10),
                    output_tokens: Some(2),
                    cache_creation_input_tokens: None,
                    cache_read_input_tokens: None,
                    cache_creation: None,
                    inference_geo: None,
                    service_tier: None,
                    server_tool_use: None,
                }),
            },
            &mut state,
        );

        assert!(state.usage_received);
    }
}
