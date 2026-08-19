use async_stream::try_stream;
use async_trait::async_trait;
use axum::http::StatusCode;
use futures_core::Stream;
use futures_util::StreamExt;
use std::collections::HashMap;
use std::pin::Pin;

use crate::llm_provider::openai_compatible::client::{ApiError, ClientError};
use crate::llm_provider::openai_compatible::{client, mapper};
use crate::llm_provider::{Provider, ProviderError, UnifiedEvent, UnifiedResponse};
use crate::openai_http_mapping::validate_openai_request;
use crate::openai_types::ChatCompletionRequest;
use crate::serve_config::ConfigError;

/// Default OrcaRouter endpoint.
pub const ORCAROUTER_DEFAULT_BASE_URL: &str = "https://api.orcarouter.ai/v1";

const CONTENT_FILTER_MESSAGE: &str =
    "The request was rejected by the safety policy. Please revise your input and try again.";

#[derive(Clone)]
pub struct OrcaRouterProvider {
    client: client::Client,
    model_id: Option<String>,
}

impl OrcaRouterProvider {
    pub fn new(client: client::Client, model_id: Option<String>) -> Self {
        Self { client, model_id }
    }
}

#[async_trait]
impl<M> Provider<M> for OrcaRouterProvider {
    fn model_id(&self) -> &str {
        self.model_id.as_deref().unwrap_or("orcarouter")
    }

    async fn complete(
        &self,
        mut request: ChatCompletionRequest,
        _metadata: &M,
    ) -> Result<UnifiedResponse, ProviderError> {
        if let Some(model_id) = &self.model_id {
            request.model = model_id.clone();
        }
        validate_request(&request)?;
        let model = request.model.clone();
        let response = self
            .client
            .chat_completions(request)
            .await
            .map_err(|err| map_openai_error(err, &model))?;
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
        validate_request(&request)?;
        let model = request.model.clone();
        let stream = self
            .client
            .chat_completions_stream(request)
            .await
            .map_err(|err| map_openai_error(err, &model))?;

        let output = try_stream! {
            futures_util::pin_mut!(stream);
            let mut state = mapper::StreamMapState::default();

            while let Some(item) = stream.next().await {
                let chunk = item.map_err(|err| map_openai_error(err, &model))?;
                for event in mapper::map_stream_chunk(chunk, &mut state) {
                    yield event;
                }
            }
        };

        Ok(Box::pin(output))
    }
}

pub fn build_orcarouter_provider(
    params: &HashMap<String, String>,
) -> Result<OrcaRouterProvider, ConfigError> {
    let api_key = params.get("api_key").cloned().unwrap_or_default();
    let mut config = client::Config::new(api_key);
    let model_id = params.get("model").cloned();
    let base_url = params
        .get("base_url")
        .cloned()
        .unwrap_or_else(|| ORCAROUTER_DEFAULT_BASE_URL.to_string());
    config = config.base_url(base_url);
    let client =
        client::Client::new(config).map_err(|err| ConfigError::InvalidProvider(err.to_string()))?;
    Ok(OrcaRouterProvider::new(client, model_id))
}

fn validate_request(request: &ChatCompletionRequest) -> Result<(), ProviderError> {
    validate_openai_request(request).map_err(ProviderError::internal)
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

#[cfg(test)]
mod tests {
    use super::*;
    use crate::openai_types::ErrorDetail;

    #[test]
    fn build_provider_defaults_to_orcarouter_base_url() {
        let provider = build_orcarouter_provider(&HashMap::new())
            .expect("orcarouter provider should build without params");
        assert_eq!(
            <OrcaRouterProvider as Provider<()>>::model_id(&provider),
            "orcarouter"
        );
    }

    #[test]
    fn build_provider_uses_configured_model_id() {
        let params = HashMap::from([("model".to_string(), "openai/gpt-4o".to_string())]);
        let provider = build_orcarouter_provider(&params)
            .expect("orcarouter provider should build with a model");
        assert_eq!(
            <OrcaRouterProvider as Provider<()>>::model_id(&provider),
            "openai/gpt-4o"
        );
    }

    #[test]
    fn build_provider_accepts_custom_base_url() {
        let params =
            HashMap::from([("base_url".to_string(), "https://example.com/v1".to_string())]);
        let provider = build_orcarouter_provider(&params)
            .expect("orcarouter provider should build with a custom base url");
        assert_eq!(
            <OrcaRouterProvider as Provider<()>>::model_id(&provider),
            "orcarouter"
        );
    }

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

        let mapped = map_openai_error(err, "orcarouter/auto");

        let ProviderError::Public { status, error } = mapped else {
            panic!("expected public error");
        };
        assert_eq!(status, axum::http::StatusCode::BAD_REQUEST);
        assert_eq!(error.message, CONTENT_FILTER_MESSAGE);
    }

    #[test]
    fn map_openai_error_rewrites_operation_not_supported() {
        let err = ClientError::Api(ApiError::OpenAI {
            status: reqwest::StatusCode::BAD_REQUEST,
            error: ErrorDetail {
                message: "original".to_string(),
                r#type: "invalid_request_error".to_string(),
                code: Some("OperationNotSupported".to_string()),
                param: None,
            },
        });

        let mapped = map_openai_error(err, "openai/gpt-5.5");

        let ProviderError::Public { error, .. } = mapped else {
            panic!("expected public error");
        };
        assert_eq!(error.code.as_deref(), Some("operation_not_supported"));
        assert_eq!(
            error.message,
            "The chatCompletion operation does not work with model openai/gpt-5.5. Please choose different model and try again."
        );
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

        let mapped = map_openai_error(err, "orcarouter/auto");

        let ProviderError::Public { error, .. } = mapped else {
            panic!("expected public error");
        };
        assert_eq!(error.message, "original");
    }
}
