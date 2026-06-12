use async_stream::try_stream;
use async_trait::async_trait;
use axum::http::StatusCode;
use futures_core::Stream;
use futures_util::StreamExt;
use std::collections::HashMap;
use std::pin::Pin;

use crate::llm_provider::openai_compatible::client::{ApiError, ClientError};
use crate::llm_provider::{Provider, ProviderError, UnifiedEvent, UnifiedResponse};
use crate::openai_http_mapping::validate_openai_request;
use crate::openai_types::{ChatCompletionRequest, ToolChoice};
use crate::serve_config::ConfigError;

pub struct OpenAIProvider {
    client: crate::llm_provider::openai_compatible::client::Client,
    model_id: Option<String>,
}

impl OpenAIProvider {
    pub fn new(
        client: crate::llm_provider::openai_compatible::client::Client,
        model_id: Option<String>,
    ) -> Self {
        Self { client, model_id }
    }
}

#[async_trait]
impl Provider for OpenAIProvider {
    fn model_id(&self) -> &str {
        "openai"
    }

    async fn complete(
        &self,
        mut request: ChatCompletionRequest,
    ) -> Result<UnifiedResponse, ProviderError> {
        if let Some(model_id) = &self.model_id {
            request.model = model_id.clone();
        }
        normalize_gpt5_request(&mut request);
        validate_openai_request(&request).map_err(ProviderError::internal)?;
        let model = request.model.clone();
        let response = self
            .client
            .chat_completions(request)
            .await
            .map_err(|err| map_openai_error(err, &model))?;

        crate::llm_provider::openai_compatible::mapper::map_response(response)
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
        normalize_gpt5_request(&mut request);
        validate_openai_request(&request).map_err(ProviderError::internal)?;
        let model = request.model.clone();
        let stream = self
            .client
            .chat_completions_stream(request)
            .await
            .map_err(|err| map_openai_error(err, &model))?;
        let stream = stream;
        let model = model.clone();

        let output = try_stream! {
            futures_util::pin_mut!(stream);
            let mut state = crate::llm_provider::openai_compatible::mapper::StreamMapState::default();

            while let Some(item) = stream.next().await {
                let chunk = item.map_err(|err| map_openai_error(err, &model))?;
                for event in crate::llm_provider::openai_compatible::mapper::map_stream_chunk(chunk, &mut state) {
                    yield event;
                }
            }
        };

        Ok(Box::pin(output))
    }
}

fn map_openai_error(err: ClientError, model: &str) -> ProviderError {
    match err {
        ClientError::Api(ApiError::OpenAI { status, mut error }) if status.as_u16() == 400 => {
            if error.code.as_deref() == Some("content_filter") {
                error.message = "The response was filtered due to the prompt triggering content management policy. Please modify your prompt and retry"
                    .to_string();
            }
            if error.code.as_deref() == Some("OperationNotSupported") {
                error.message = format!(
                    "The chatCompletion operation does not work with model {model}. Please choose different model and try again."
                );
                error.code = Some("operation_not_supported".to_string());
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

pub fn build_openai_provider(
    params: &HashMap<String, String>,
) -> Result<OpenAIProvider, ConfigError> {
    let api_key = params
        .get("api_key")
        .ok_or_else(|| ConfigError::InvalidProvider("api_key is required".to_string()))?;
    let mut config =
        crate::llm_provider::openai_compatible::client::Config::new(api_key.to_string());
    let model_id = params.get("model").cloned();
    if let Some(base_url) = params.get("base_url") {
        config = config.base_url(base_url.to_string());
    }
    let client = crate::llm_provider::openai_compatible::client::Client::new(config)
        .map_err(|err| ConfigError::InvalidProvider(err.to_string()))?;
    Ok(OpenAIProvider::new(client, model_id))
}

fn normalize_gpt5_request(request: &mut ChatCompletionRequest) {
    if !is_target_gpt5_model(&request.model) {
        return;
    }

    request.temperature = None;
    request.top_p = None;
    request.presence_penalty = None;
    request.frequency_penalty = None;
    request.logprobs = None;
    request.top_logprobs = None;

    if request.reasoning_effort.is_some() && has_tool_usage(request) {
        request.reasoning_effort = None;
    }
}

fn is_target_gpt5_model(model: &str) -> bool {
    matches!(
        model,
        "gpt-5.5" | "gpt-5.5-1" | "gpt-5.4" | "gpt-5.4-mini" | "gpt-5.4-nano"
    )
}

fn has_tool_usage(request: &ChatCompletionRequest) -> bool {
    if request
        .tools
        .as_ref()
        .is_some_and(|tools| !tools.is_empty())
    {
        return true;
    }

    if let Some(tool_choice) = &request.tool_choice {
        let uses_tools = match tool_choice {
            ToolChoice::Name(name) => name != "none",
            ToolChoice::Object { .. } => true,
        };
        if uses_tools {
            return true;
        }
    }

    request.messages.iter().any(|message| {
        message
            .tool_calls
            .as_ref()
            .is_some_and(|calls| !calls.is_empty())
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::openai_types::{
        Content, ErrorDetail, FunctionDefinition, Message, Role, ToolCall, ToolCallFunction,
        ToolDefinition,
    };
    use serde_json::json;

    fn base_request(model: &str) -> ChatCompletionRequest {
        ChatCompletionRequest {
            model: model.to_string(),
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
    fn normalize_gpt5_clears_unsupported_params() {
        let mut request = base_request("gpt-5.5");
        request.temperature = Some(0.7);
        request.top_p = Some(0.9);
        request.presence_penalty = Some(0.1);
        request.frequency_penalty = Some(0.2);
        request.logprobs = Some(true);
        request.top_logprobs = Some(5);

        normalize_gpt5_request(&mut request);

        assert!(request.temperature.is_none());
        assert!(request.top_p.is_none());
        assert!(request.presence_penalty.is_none());
        assert!(request.frequency_penalty.is_none());
        assert!(request.logprobs.is_none());
        assert!(request.top_logprobs.is_none());
    }

    #[test]
    fn normalize_non_target_model_keeps_params() {
        let mut request = base_request("gpt-4");
        request.temperature = Some(0.7);
        request.top_p = Some(0.9);

        normalize_gpt5_request(&mut request);

        assert_eq!(request.temperature, Some(0.7));
        assert_eq!(request.top_p, Some(0.9));
    }

    #[test]
    fn normalize_gpt5_clears_reasoning_effort_when_tools_present() {
        let mut request = base_request("gpt-5.4-mini");
        request.reasoning_effort = Some("medium".to_string());
        request.tools = Some(vec![ToolDefinition {
            r#type: "function".to_string(),
            function: FunctionDefinition {
                name: "test_tool".to_string(),
                description: None,
                strict: None,
                parameters: json!({}),
            },
        }]);

        normalize_gpt5_request(&mut request);

        assert!(request.reasoning_effort.is_none());
    }

    #[test]
    fn normalize_gpt5_clears_reasoning_effort_for_message_tool_calls() {
        let mut request = base_request("gpt-5.4-nano");
        request.reasoning_effort = Some("low".to_string());
        request.messages.push(Message {
            role: Role::Assistant,
            content: Content::Text("".to_string()),
            reasoning_content: None,
            tool_call_id: None,
            tool_calls: Some(vec![ToolCall {
                id: Some("call-1".to_string()),
                r#type: Some("function".to_string()),
                function: ToolCallFunction {
                    name: "test_tool".to_string(),
                    arguments: "{}".to_string(),
                    description: None,
                },
            }]),
        });

        normalize_gpt5_request(&mut request);

        assert!(request.reasoning_effort.is_none());
    }

    #[test]
    fn map_openai_error_rewrites_content_filter_message() {
        let error = ErrorDetail {
            message: "original".to_string(),
            r#type: "invalid_request_error".to_string(),
            code: Some("content_filter".to_string()),
            param: None,
        };
        let err = ClientError::Api(ApiError::OpenAI {
            status: StatusCode::BAD_REQUEST,
            error,
        });

        let mapped = map_openai_error(err, "gpt-5.5");

        let ProviderError::Public { error, .. } = mapped else {
            panic!("expected public error");
        };
        assert_eq!(
            error.message,
            "The response was filtered due to the prompt triggering content management policy. Please modify your prompt and retry"
        );
    }

    #[test]
    fn map_openai_error_rewrites_operation_not_supported() {
        let error = ErrorDetail {
            message: "original".to_string(),
            r#type: "invalid_request_error".to_string(),
            code: Some("OperationNotSupported".to_string()),
            param: None,
        };
        let err = ClientError::Api(ApiError::OpenAI {
            status: StatusCode::BAD_REQUEST,
            error,
        });

        let mapped = map_openai_error(err, "gpt-5.4");

        let ProviderError::Public { error, .. } = mapped else {
            panic!("expected public error");
        };
        assert_eq!(error.code.as_deref(), Some("operation_not_supported"));
        assert_eq!(
            error.message,
            "The chatCompletion operation does not work with model gpt-5.4. Please choose different model and try again."
        );
    }
}
