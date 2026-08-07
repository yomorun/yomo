use std::collections::HashMap;
use std::pin::Pin;

use async_stream::try_stream;
use async_trait::async_trait;
use futures_core::Stream;
use futures_util::StreamExt;
use log::warn;

use crate::llm_provider::{Provider, ProviderError, UnifiedEvent, UnifiedResponse};
use crate::openai_http_mapping::validate_openai_request;
use crate::openai_types::ChatCompletionRequest;
use crate::serve_config::ConfigError;

use self::client::{Backend, BedrockClient, DirectClient, map_client_error, parse_auth_style};
use self::mapper::{map_request, map_response, map_stop_reason_string, map_stream_event};
use self::types::{
    AnthropicRequest, BedrockRequest, DEFAULT_ANTHROPIC_VERSION, DEFAULT_BEDROCK_ANTHROPIC_VERSION,
    DEFAULT_MAX_TOKENS, StreamState,
};

mod client;
mod mapper;
mod types;

#[derive(Clone)]
pub struct AnthropicMessagesProvider {
    backend: Backend,
    upstream_model: String,
    anthropic_version: String,
    default_max_tokens: i32,
}

impl AnthropicMessagesProvider {
    fn new(
        backend: Backend,
        upstream_model: String,
        anthropic_version: String,
        default_max_tokens: i32,
    ) -> Self {
        Self {
            backend,
            upstream_model,
            anthropic_version,
            default_max_tokens,
        }
    }
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
        let mapped = map_request(
            request,
            self.upstream_model.clone(),
            self.default_max_tokens,
            true,
        )?;
        let response = match &self.backend {
            Backend::Direct(client) => {
                let payload = AnthropicRequest {
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
                };
                client
                    .send_complete(payload, &self.anthropic_version)
                    .await
                    .map_err(map_client_error)?
            }
            Backend::Bedrock(client) => {
                let payload = BedrockRequest {
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
                };
                client
                    .send_complete(payload)
                    .await
                    .map_err(map_client_error)?
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
        let mapped = map_request(
            request,
            self.upstream_model.clone(),
            self.default_max_tokens,
            true,
        )?;

        let stream = match &self.backend {
            Backend::Direct(client) => {
                let payload = AnthropicRequest {
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
                };
                client
                    .send_stream(payload, &self.anthropic_version)
                    .await
                    .map_err(map_client_error)?
            }
            Backend::Bedrock(client) => {
                let payload = BedrockRequest {
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
                };
                client
                    .send_stream(payload)
                    .await
                    .map_err(map_client_error)?
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
        DEFAULT_MAX_TOKENS,
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
    let max_tokens = params
        .get("max_tokens")
        .and_then(|raw| raw.parse::<i32>().ok())
        .unwrap_or(DEFAULT_MAX_TOKENS);

    Ok(AnthropicMessagesProvider::new(
        Backend::Bedrock(BedrockClient {
            model_id: bedrock_model.clone(),
            aws_region,
            aws_bearer_token,
        }),
        bedrock_model,
        anthropic_version,
        max_tokens,
    ))
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;
    use crate::llm_provider::anthropic_messages::types::{
        AnthropicContentBlock, AnthropicThinking,
    };
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

        assert_eq!(provider.default_max_tokens, DEFAULT_MAX_TOKENS);
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

        let mapped = map_request(
            request,
            "claude-sonnet-5".to_string(),
            DEFAULT_MAX_TOKENS,
            false,
        )
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

        let mapped = map_request(
            request,
            "claude-sonnet-5".to_string(),
            DEFAULT_MAX_TOKENS,
            false,
        )
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

        let mapped = map_request(
            request,
            "claude-sonnet-5".to_string(),
            DEFAULT_MAX_TOKENS,
            false,
        )
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

        let mapped = map_request(
            request,
            "claude-haiku-4-5".to_string(),
            DEFAULT_MAX_TOKENS,
            false,
        )
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
            DEFAULT_MAX_TOKENS,
            false,
        )
        .expect("request should map");

        assert!(mapped.thinking.is_none());
    }

    #[test]
    fn map_request_uses_adaptive_thinking_from_reasoning_effort_for_non_haiku_models() {
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
            DEFAULT_MAX_TOKENS,
            false,
        )
        .expect("request should map");

        assert!(matches!(mapped.thinking, Some(AnthropicThinking::Adaptive)));
    }

    #[test]
    fn map_request_drops_non_default_sampling_for_restricted_models_when_compat_enabled() {
        let mut request = basic_chat_request(Some(0.2), Some(0.7));
        request.model = "claude-sonnet-5".to_string();

        let mapped = map_request(
            request,
            "claude-sonnet-5".to_string(),
            DEFAULT_MAX_TOKENS,
            true,
        )
        .expect("request should map");

        assert_eq!(mapped.temperature, None);
        assert_eq!(mapped.top_p, None);
    }

    #[test]
    fn map_request_keeps_non_default_sampling_for_non_restricted_models_when_compat_enabled() {
        let request = basic_chat_request(Some(0.2), Some(0.7));

        let mapped = map_request(
            request,
            "claude-sonnet-4-6".to_string(),
            DEFAULT_MAX_TOKENS,
            true,
        )
        .expect("request should map");

        assert_eq!(mapped.temperature, Some(0.2));
        assert_eq!(mapped.top_p, Some(0.7));
    }

    #[test]
    fn map_request_sampling_compat_checks_request_model_only() {
        let mut request = basic_chat_request(Some(0.2), Some(0.7));
        request.model = "global.anthropic.claude-sonnet-5".to_string();

        let mapped = map_request(
            request,
            "claude-sonnet-5".to_string(),
            DEFAULT_MAX_TOKENS,
            true,
        )
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

        let mapped = map_request(
            request,
            "claude-sonnet-5".to_string(),
            DEFAULT_MAX_TOKENS,
            false,
        )
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
