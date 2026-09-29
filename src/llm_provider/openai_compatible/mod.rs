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
}

impl OpenAICompatibleProvider {
    pub fn new(client: client::Client, model_id: Option<String>) -> Self {
        Self {
            client,
            model_id,
            system_prompt: None,
        }
    }

    pub fn with_system_prompt(mut self, system_prompt: Option<String>) -> Self {
        self.system_prompt = system_prompt;
        self
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
        if let Some(model_id) = &self.model_id {
            request.model = model_id.clone();
        }
        if let Some(system_prompt) = &self.system_prompt {
            prepend_system_prompt(&mut request, system_prompt);
        }
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
        if let Some(system_prompt) = &self.system_prompt {
            prepend_system_prompt(&mut request, system_prompt);
        }
        validate_request(&request)?;
        let stream = self
            .client
            .chat_completions_stream(request)
            .await
            .map_err(map_openai_error)?;
        let stream = stream;

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
    if let Some(base_url) = params.get("base_url") {
        config = config.base_url(base_url.to_string());
    }
    let client =
        client::Client::new(config).map_err(|err| ConfigError::InvalidProvider(err.to_string()))?;
    Ok(OpenAICompatibleProvider::new(client, model_id).with_system_prompt(system_prompt))
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
    use super::map_openai_error;
    use super::prepend_system_prompt;
    use std::collections::HashMap;

    use crate::llm_provider::ProviderError;
    use crate::llm_provider::openai_compatible::client::{ApiError, ClientError};
    use crate::openai_types::ChatCompletionRequest;
    use crate::openai_types::{Content, ErrorDetail, Message, Role};

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
