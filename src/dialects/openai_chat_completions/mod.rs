//! OpenAI Chat request/response transport. Vendor behavior is selected explicitly
//! by upstream bindings, never inferred from URLs or deployment-name prefixes.

pub mod client;
pub mod mapper;

use std::pin::Pin;
use std::sync::Arc;

use async_stream::try_stream;
use async_trait::async_trait;
use futures_core::Stream;
use futures_util::StreamExt;
use secrecy::ExposeSecret;

use crate::dialects::credential::{shared_http_client, shared_http1_client};
use crate::models::{ModelProfile, default_model_profile};
use crate::openai_http_mapping::{
    RESPONSES_NORMALIZE_NON_FUNCTION_TOOLS_MARKER, validate_openai_request,
};
use crate::openai_types::{ChatCompletionRequest, ErrorDetail};
use crate::provider::{Provider, ProviderError, UnifiedEvent, UnifiedRequest, UnifiedResponse};
use crate::serve_config::{HttpVersion, ModelBinding, OpenAiParams};
use client::{ApiError, ClientError, Config};

/// Chat dialect client with an optional explicit upstream binding.
#[derive(Clone)]
pub struct OpenAiChatCompletionsProvider {
    client: client::Client,
    model: String,
    profile: Arc<dyn ModelProfile>,
}

impl OpenAiChatCompletionsProvider {
    /// Construct a generic Chat client without vendor inference or normalization.
    pub fn new(params: &OpenAiParams) -> Self {
        Self::construct(params, None, default_model_profile())
    }

    /// Construct a startup-bound Chat client using vendor rules.
    pub fn new_bound(params: &OpenAiParams, binding: ModelBinding) -> Self {
        Self::new_bound_with_profile(params, binding, default_model_profile())
    }

    /// Construct a startup-bound Chat client with a caller-provided model profile.
    pub fn new_bound_with_profile(
        params: &OpenAiParams,
        binding: ModelBinding,
        profile: Arc<dyn ModelProfile>,
    ) -> Self {
        Self::construct(params, Some(binding), profile)
    }

    fn construct(
        params: &OpenAiParams,
        binding: Option<ModelBinding>,
        profile: Arc<dyn ModelProfile>,
    ) -> Self {
        let api_key = params
            .auth
            .as_ref()
            .map(|auth| auth.api_key.expose_secret().to_owned())
            .unwrap_or_default();
        let mut config = Config::new(api_key).base_url(params.base_url.as_str());
        config.binding = binding;
        if let Some(auth) = &params.auth {
            config.auth_scheme = auth.scheme;
        }
        let http = match params.http_version {
            Some(HttpVersion::Http11) => shared_http1_client(),
            None => shared_http_client(),
        };
        Self {
            client: client::Client::with_client(config, http),
            model: params.model.clone(),
            profile,
        }
    }

    fn unified_to_request(
        &self,
        request: &UnifiedRequest,
    ) -> Result<ChatCompletionRequest, ProviderError> {
        let request = match self.client.config.binding.as_ref() {
            Some(binding) => match self.profile.to_openai_chat_request(request, binding)? {
                Some(request) => request,
                None => mapper::request_to_chat(request)?,
            },
            None => mapper::request_to_chat(request)?,
        };
        self.prepare_request(request)
    }

    fn prepare_request(
        &self,
        mut request: ChatCompletionRequest,
    ) -> Result<ChatCompletionRequest, ProviderError> {
        request.model = self.model.clone();
        request
            .extra
            .remove(RESPONSES_NORMALIZE_NON_FUNCTION_TOOLS_MARKER);
        validate_openai_request(&request).map_err(ProviderError::internal)?;
        Ok(request)
    }
}

fn map_error(error: ClientError) -> ProviderError {
    match error {
        ClientError::Api(ApiError::Upstream(error)) => ProviderError::Public {
            status: error.status,
            error: ErrorDetail {
                message: error.message,
                r#type: error.error_type.unwrap_or_else(|| "upstream_error".into()),
                code: error.code,
                param: error.param,
            },
        },
        ClientError::Api(ApiError::OpenAI { status, error }) => {
            ProviderError::Public { status, error }
        }
        ClientError::Api(ApiError::Unknown { status, body }) => {
            ProviderError::internal_with_upstream_status(status, body)
        }
        ClientError::Api(ApiError::Custom { status, value }) => {
            ProviderError::internal_with_upstream_status(
                status.unwrap_or_default(),
                value.to_string(),
            )
        }
        error => ProviderError::internal(error.to_string()),
    }
}

#[async_trait]
impl<M> Provider<M> for OpenAiChatCompletionsProvider {
    fn model_id(&self) -> &str {
        &self.model
    }
    fn binding(&self) -> Option<&ModelBinding> {
        self.client.config.binding.as_ref()
    }

    async fn complete(&self, request: UnifiedRequest) -> Result<UnifiedResponse, ProviderError> {
        let transport_headers = request.transport_headers.clone();
        let request = self.unified_to_request(&request)?;
        mapper::map_response(
            self.client
                .chat_completions(request, &transport_headers)
                .await
                .map_err(map_error)?,
        )
    }

    async fn stream<'a>(
        &'a self,
        request: UnifiedRequest,
    ) -> Result<
        Pin<Box<dyn Stream<Item = Result<UnifiedEvent, ProviderError>> + Send + 'a>>,
        ProviderError,
    > {
        let transport_headers = request.transport_headers.clone();
        let request = self.unified_to_request(&request)?;
        let stream = self
            .client
            .chat_completions_stream(request, &transport_headers)
            .await
            .map_err(map_error)?;
        Ok(Box::pin(try_stream! {
            futures_util::pin_mut!(stream);
            let mut state = mapper::StreamMapState::default();
            while let Some(item) = stream.next().await {
                for event in mapper::map_stream_chunk(item.map_err(map_error)?, &mut state) { yield event; }
            }
        }))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::serve_config::{AuthConfig, AuthScheme};
    use secrecy::SecretString;
    use serde_json::Value;

    #[test]
    fn chat_encoding_preserves_effort_sampling_and_extensions_regardless_of_url() {
        let provider = OpenAiChatCompletionsProvider::new(&OpenAiParams {
            base_url: "https://example.openai.azure.com/openai/v1"
                .parse()
                .expect("URL"),
            auth: Some(AuthConfig {
                scheme: AuthScheme::ApiKey,
                api_key: SecretString::from("test"),
            }),
            model: "gpt-5.6-sol".into(),
            response_model: None,
            http_version: None,
        });
        let mut request = ChatCompletionRequest {
            messages: serde_json::from_value(
                serde_json::json!([{"role":"user","content":"hello"}]),
            )
            .expect("messages"),
            reasoning_effort: Some("max".into()),
            temperature: Some(0.7),
            ..Default::default()
        };
        request
            .extra
            .insert("tool_stream".into(), Value::Bool(true));
        let prepared = provider.prepare_request(request).expect("prepared");
        assert_eq!(
            prepared
                .reasoning_effort
                .as_ref()
                .and_then(|effort| effort.as_str()),
            Some("max")
        );
        assert_eq!(prepared.temperature, Some(0.7));
        assert_eq!(prepared.extra.get("tool_stream"), Some(&Value::Bool(true)));
        assert_eq!(prepared.model, "gpt-5.6-sol");
    }
}
