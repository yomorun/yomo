use async_stream::try_stream;
use futures_core::Stream;
use futures_util::StreamExt;
use std::pin::Pin;

use crate::serve_config::ConfigError;
use crate::openai_http_mapping::validate_openai_request;
use crate::openai_types::{ChatCompletionRequest, ClientError};
use crate::llm_provider::{Provider, ProviderError, UnifiedEvent, UnifiedResponse};

mod client;

pub mod mapper;

#[derive(Clone)]
pub struct OpenAIProvider {
    client: client::Client,
    model_id: Option<String>,
}

impl OpenAIProvider {
    pub fn new(client: client::Client, model_id: Option<String>) -> Self {
        Self { client, model_id }
    }
}

impl Provider for OpenAIProvider {
    fn model_id(&self) -> &str {
        "openai"
    }

    fn complete<'a>(
        &'a self,
        mut request: ChatCompletionRequest,
    ) -> Pin<
        Box<dyn futures_core::Future<Output = Result<UnifiedResponse, ProviderError>> + Send + 'a>,
    >
    {
        Box::pin(async move {
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
        })
    }

    fn stream<'a>(
        &'a self,
        mut request: ChatCompletionRequest,
    ) -> Pin<Box<dyn Stream<Item = Result<UnifiedEvent, ProviderError>> + Send + 'a>> {
        Box::pin(try_stream! {
            if let Some(model_id) = &self.model_id {
                request.model = model_id.clone();
            }
            validate_request(&request)?;
            let stream = self
                .client
                .chat_completions_stream(request)
                .await
                .map_err(map_openai_error)?;
            futures_util::pin_mut!(stream);
            let mut state = mapper::StreamMapState::default();

            while let Some(item) = stream.next().await {
                let chunk = item.map_err(map_openai_error)?;
                for event in mapper::map_stream_chunk(chunk, &mut state) {
                    yield event;
                }
            }
        })
    }
}

fn map_openai_error(err: ClientError) -> ProviderError {
    ProviderError::Internal(err.to_string())
}

pub fn build_openai_provider(
    params: &std::collections::HashMap<String, String>,
) -> Result<OpenAIProvider, ConfigError> {
    let api_key = params
        .get("api_key")
        .ok_or_else(|| ConfigError::InvalidProvider("api_key is required".to_string()))?;
    let mut config = client::Config::new(api_key.to_string());
    let model_id = params.get("model").cloned();
    if let Some(base_url) = params.get("base_url") {
        config = config.base_url(base_url.to_string());
    }
    let client = client::Client::new(config)
        .map_err(|err| ConfigError::InvalidProvider(err.to_string()))?;
    Ok(OpenAIProvider::new(client, model_id))
}

fn validate_request(request: &ChatCompletionRequest) -> Result<(), ProviderError> {
    validate_openai_request(request).map_err(ProviderError::Internal)
}
