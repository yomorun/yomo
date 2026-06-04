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
use crate::openai_types::ChatCompletionRequest;
use crate::serve_config::ConfigError;

pub mod client;

pub mod mapper;

#[derive(Clone)]
pub struct OpenAICompatibleProvider {
    client: client::Client,
    model_id: Option<String>,
}

impl OpenAICompatibleProvider {
    pub fn new(client: client::Client, model_id: Option<String>) -> Self {
        Self { client, model_id }
    }
}

#[async_trait]
impl Provider for OpenAICompatibleProvider {
    fn model_id(&self) -> &str {
        "openai-compatible"
    }

    async fn complete(
        &self,
        mut request: ChatCompletionRequest,
    ) -> Result<UnifiedResponse, ProviderError> {
        if let Some(model_id) = &self.model_id {
            request.model = model_id.clone();
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
    ) -> Result<
        Pin<Box<dyn Stream<Item = Result<UnifiedEvent, ProviderError>> + Send + 'a>>,
        ProviderError,
    > {
        if let Some(model_id) = &self.model_id {
            request.model = model_id.clone();
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
        ClientError::Api(ApiError::OpenAI { status, error }) if status.as_u16() == 400 => {
            ProviderError::Public {
                status: StatusCode::BAD_REQUEST,
                error,
            }
        }
        other => ProviderError::Internal(other.to_string()),
    }
}

pub fn build_openai_compatible_provider(
    params: &HashMap<String, String>,
) -> Result<OpenAICompatibleProvider, ConfigError> {
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
    Ok(OpenAICompatibleProvider::new(client, model_id))
}

fn validate_request(request: &ChatCompletionRequest) -> Result<(), ProviderError> {
    validate_openai_request(request).map_err(ProviderError::Internal)
}
