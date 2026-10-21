use async_stream::try_stream;
use async_trait::async_trait;
use axum::http::StatusCode;
use futures_core::Stream;
use futures_util::StreamExt;
use log::warn;
use std::collections::HashMap;
use std::pin::Pin;
use std::sync::atomic::{AtomicBool, Ordering};

use crate::llm_provider::openai_compatible::client::{ApiError, ClientError};
use crate::llm_provider::openai_compatible::{
    is_responses_unsupported, parse_upstream_api_mode, responses_event_stream,
};
use crate::llm_provider::openai_responses::{
    UpstreamApiMode, can_fallback_to_chat_completions, chat_to_responses_request,
    map_responses_to_unified, should_route_to_responses,
};
use crate::llm_provider::{Provider, ProviderError, UnifiedEvent, UnifiedResponse};
use crate::openai_http_mapping::validate_openai_request;
use crate::openai_types::{ChatCompletionRequest, ToolChoice};
use crate::serve_config::ConfigError;

pub struct OpenAIProvider {
    client: crate::llm_provider::openai_compatible::client::Client,
    model_id: Option<String>,
    upstream_api: UpstreamApiMode,
    /// Set once the upstream rejected the Responses API, so later requests skip
    /// the failed Responses attempt.
    responses_unsupported: AtomicBool,
}

const CONTENT_FILTER_MESSAGE: &str =
    "The request was rejected by the safety policy. Please revise your input and try again.";

impl OpenAIProvider {
    pub fn new(
        client: crate::llm_provider::openai_compatible::client::Client,
        model_id: Option<String>,
    ) -> Self {
        Self {
            client,
            model_id,
            upstream_api: UpstreamApiMode::default(),
            responses_unsupported: AtomicBool::new(false),
        }
    }

    /// Overrides how the upstream API is selected for a request.
    pub fn with_upstream_api(mut self, upstream_api: UpstreamApiMode) -> Self {
        self.upstream_api = upstream_api;
        self
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
impl<M> Provider<M> for OpenAIProvider {
    fn model_id(&self) -> &str {
        "openai"
    }

    async fn complete(
        &self,
        mut request: ChatCompletionRequest,
        _metadata: &M,
    ) -> Result<UnifiedResponse, ProviderError> {
        if let Some(model_id) = &self.model_id {
            request.model = model_id.clone();
        }
        validate_openai_request(&request).map_err(ProviderError::internal)?;
        let model = request.model.clone();

        if self.should_use_responses(&model) {
            let translated = chat_to_responses_request(&request)?;
            match self.client.responses_completions(translated).await {
                Ok(response) => return map_responses_to_unified(response),
                // Unsupported upstream: replay through Chat Completions.
                Err(err) if self.can_fallback() && is_responses_unsupported(&err) => {
                    self.responses_unsupported.store(true, Ordering::Relaxed);
                    warn!("responses upstream unsupported, replaying via chat completions: {err}");
                }
                Err(err) => return Err(map_openai_error(err, &model)),
            }
        }

        normalize_gpt5_request(&mut request);
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
        _metadata: &M,
    ) -> Result<
        Pin<Box<dyn Stream<Item = Result<UnifiedEvent, ProviderError>> + Send + 'a>>,
        ProviderError,
    > {
        if let Some(model_id) = &self.model_id {
            request.model = model_id.clone();
        }
        validate_openai_request(&request).map_err(ProviderError::internal)?;
        let model = request.model.clone();

        if self.should_use_responses(&model) {
            let translated = chat_to_responses_request(&request)?;
            match self.client.responses_completions_stream(translated).await {
                Ok(events) => return Ok(Box::pin(responses_event_stream(events))),
                // Unsupported upstream: replay through Chat Completions.
                Err(err) if self.can_fallback() && is_responses_unsupported(&err) => {
                    self.responses_unsupported.store(true, Ordering::Relaxed);
                    warn!("responses upstream unsupported, replaying via chat completions: {err}");
                }
                Err(err) => return Err(map_openai_error(err, &model)),
            }
        }

        normalize_gpt5_request(&mut request);
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
                error.message = CONTENT_FILTER_MESSAGE.to_string();
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
    let upstream_api = parse_upstream_api_mode(params.get("upstream_api"))?;
    Ok(OpenAIProvider::new(client, model_id).with_upstream_api(upstream_api))
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

    if !has_tool_usage(request) {
        return;
    }

    if requires_none_reasoning_with_tools(&request.model) {
        // GPT-5.6 and later only accept function calling on Chat Completions
        // with `reasoning_effort: "none"`. The API defaults to `medium`, so
        // always set it explicitly, even when the caller omitted it.
        request.reasoning_effort = Some("none".to_string());
    } else if request.reasoning_effort.is_some() {
        // Earlier models reject an explicit reasoning effort together with
        // tools, so omit it and let the provider use its default.
        request.reasoning_effort = None;
    }
}

/// GPT-5.6 and later models that only support function calling on Chat
/// Completions with `reasoning_effort: "none"`.
///
/// `gpt-6-astra` and `gpt-6.1-sol` are intentionally excluded: they do not
/// support the `none` effort, and Chat Completions does not support function
/// calling for them at all, so sending `none` would not make the request valid.
fn requires_none_reasoning_with_tools(model: &str) -> bool {
    matches!(
        model,
        "gpt-5.6" | "gpt-5.6-sol" | "gpt-5.6-luna" | "gpt-5.6-terra" | "gpt-6-sol" | "gpt-6-luna"
    )
}

fn is_target_gpt5_model(model: &str) -> bool {
    matches!(
        model,
        "gpt-5.5"
            | "gpt-5.5-1"
            | "gpt-5.4"
            | "gpt-5.4-mini"
            | "gpt-5.4-nano"
            | "gpt-5.6"
            | "gpt-5.6-sol"
            | "gpt-5.6-luna"
            | "gpt-5.6-terra"
            | "gpt-6-astra"
            | "gpt-6.1-sol"
            | "gpt-6-sol"
            | "gpt-6-luna"
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
    fn normalize_gpt56_and_later_forces_reasoning_effort_none_with_tools() {
        for model in [
            "gpt-5.6",
            "gpt-5.6-sol",
            "gpt-5.6-luna",
            "gpt-5.6-terra",
            "gpt-6-sol",
            "gpt-6-luna",
        ] {
            let mut request = base_request(model);
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

            assert_eq!(
                request.reasoning_effort.as_deref(),
                Some("none"),
                "expected {model} to force reasoning_effort none"
            );
        }
    }

    #[test]
    fn normalize_gpt56_and_later_forces_none_when_effort_is_omitted() {
        for model in [
            "gpt-5.6-sol",
            "gpt-5.6-luna",
            "gpt-5.6-terra",
            "gpt-6-sol",
            "gpt-6-luna",
        ] {
            let mut request = base_request(model);
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

            assert_eq!(
                request.reasoning_effort.as_deref(),
                Some("none"),
                "expected {model} to default to none with tools"
            );
        }
    }

    #[test]
    fn normalize_gpt6_astra_and_gpt61_sol_do_not_force_none_with_tools() {
        for model in ["gpt-6-astra", "gpt-6.1-sol"] {
            let mut request = base_request(model);
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

            // `none` is not supported by these models, so the effort is
            // dropped rather than forced.
            assert!(
                request.reasoning_effort.is_none(),
                "expected {model} not to force reasoning_effort none"
            );
        }
    }

    #[test]
    fn normalize_gpt56_and_later_keeps_reasoning_effort_without_tools() {
        for model in [
            "gpt-5.6-sol",
            "gpt-5.6-luna",
            "gpt-5.6-terra",
            "gpt-6-sol",
            "gpt-6-luna",
        ] {
            let mut request = base_request(model);
            request.reasoning_effort = Some("medium".to_string());

            normalize_gpt5_request(&mut request);

            assert_eq!(request.reasoning_effort.as_deref(), Some("medium"));
        }
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
    fn is_target_gpt5_model_includes_gpt56_and_gpt6_astra() {
        for model in [
            "gpt-5.6",
            "gpt-5.6-sol",
            "gpt-5.6-luna",
            "gpt-5.6-terra",
            "gpt-6-sol",
            "gpt-6-luna",
        ] {
            assert!(
                is_target_gpt5_model(model),
                "expected {model} to be a target"
            );
        }

        assert!(is_target_gpt5_model("gpt-6-astra"));
        assert!(is_target_gpt5_model("gpt-6.1-sol"));

        assert!(!is_target_gpt5_model("gpt-5.6-unknown"));
        assert!(!is_target_gpt5_model("gpt-6-astra-plus"));
        assert!(!is_target_gpt5_model("gpt-6-sol-plus"));
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
        assert_eq!(error.message, CONTENT_FILTER_MESSAGE);
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

    #[test]
    fn build_openai_provider_reads_upstream_api_param() {
        let params: HashMap<String, String> = HashMap::from([
            ("api_key".to_string(), "key".to_string()),
            ("upstream_api".to_string(), "responses".to_string()),
        ]);

        let provider = build_openai_provider(&params).expect("provider builds");

        assert_eq!(provider.upstream_api, UpstreamApiMode::Responses);
    }

    #[test]
    fn build_openai_provider_defaults_upstream_api_to_auto() {
        let params: HashMap<String, String> =
            HashMap::from([("api_key".to_string(), "key".to_string())]);

        let provider = build_openai_provider(&params).expect("provider builds");

        assert_eq!(provider.upstream_api, UpstreamApiMode::Auto);
    }

    #[test]
    fn build_openai_provider_rejects_invalid_upstream_api() {
        let params: HashMap<String, String> = HashMap::from([
            ("api_key".to_string(), "key".to_string()),
            ("upstream_api".to_string(), "grpc".to_string()),
        ]);

        let error = match build_openai_provider(&params) {
            Ok(_) => panic!("expected invalid upstream_api to be rejected"),
            Err(error) => error,
        };

        assert!(error.to_string().contains("invalid upstream_api"));
    }
}
