//! OpenAI Responses upstream provider and HTTP lifecycle.

pub mod mapper;

use mapper::upstream_error;
pub use mapper::{StreamMapState, ir_to_responses_request, map_stream_event, responses_to_unified};

use std::pin::Pin;
use std::time::Duration;

use async_stream::try_stream;
use async_trait::async_trait;
use futures_core::Stream;
use futures_util::StreamExt;
use secrecy::SecretString;
use tokio::time::timeout;
use url::Url;

use crate::api::{request_to_value, response_from_value};
use crate::dialects::credential::shared_http_client;
use crate::openai_types::ErrorDetail;
use crate::provider::UnifiedRequest;
use crate::provider::UnifiedSourceProtocol;
use crate::provider::UpstreamLog;
use crate::provider::{Provider, ProviderError, UnifiedEvent, UnifiedResponse};
use crate::serve_config::{AuthScheme, ModelBinding, OpenAiParams};
use crate::upstreams::{Dialect, parse_error, request_path};
use crate::utils::SseDataReader;
use axum::http::{HeaderMap, StatusCode};
use reqwest::RequestBuilder;
use serde_json::{Value, json};

const STREAM_FIRST_BYTE_TIMEOUT_SECS: u64 = 180;
const STREAM_IDLE_TIMEOUT_SECS: u64 = 180;

/// `openai-responses` dialect client.
#[derive(Clone)]
pub struct OpenAiResponsesProvider {
    http: reqwest::Client,
    base_url: Url,
    api_key: Option<SecretString>,
    /// Upstream model / deployment name (params.model).
    model: String,
    binding: Option<ModelBinding>,
    auth_scheme: AuthScheme,
}

impl OpenAiResponsesProvider {
    pub fn new(params: &OpenAiParams) -> Self {
        Self {
            http: shared_http_client(),
            base_url: params.base_url.clone(),
            api_key: params.auth.as_ref().map(|auth| auth.api_key.clone()),
            model: params.model.clone(),
            binding: None,
            auth_scheme: params
                .auth
                .as_ref()
                .map(|auth| auth.scheme)
                .unwrap_or(AuthScheme::Bearer),
        }
    }

    /// Create a fixed integration with no URL-based reasoning normalization.
    pub fn new_bound(params: &OpenAiParams, binding: ModelBinding) -> Self {
        let mut provider = Self::new(params);
        provider.binding = Some(binding);
        provider
    }

    fn apply_auth(&self, request: RequestBuilder) -> RequestBuilder {
        use secrecy::ExposeSecret;
        match (&self.api_key, self.auth_scheme) {
            (Some(key), AuthScheme::ApiKey) => request.header("api-key", key.expose_secret()),
            _ => mapper::apply_optional_bearer_auth(request, self.api_key.as_ref()),
        }
    }

    fn parse_failure(&self, status: StatusCode, headers: &HeaderMap, body: &[u8]) -> ProviderError {
        if let Some(binding) = &self.binding {
            let error = parse_error(binding.upstream, status, headers, body);
            ProviderError::Public {
                status: error.status,
                error: ErrorDetail {
                    message: error.message,
                    r#type: error.error_type.unwrap_or_else(|| "upstream_error".into()),
                    code: error.code,
                    param: error.param,
                },
            }
        } else {
            upstream_error(status, body)
        }
    }

    fn url(&self) -> String {
        format!(
            "{}{}",
            self.base_url.as_str().trim_end_matches('/'),
            request_path(Dialect::OpenAiResponses)
        )
    }
}

#[async_trait]
impl<M> Provider<M> for OpenAiResponsesProvider {
    fn model_id(&self) -> &str {
        &self.model
    }

    fn binding(&self) -> Option<&ModelBinding> {
        self.binding.as_ref()
    }

    async fn complete(
        &self,
        request: UnifiedRequest,
        _metadata: &M,
    ) -> Result<UnifiedResponse, ProviderError> {
        let mut body = request_to_value(&request, UnifiedSourceProtocol::Responses)
            .map_err(ProviderError::internal)?;
        body["model"] = json!(self.model);
        let diagnostics = UpstreamLog::new("openai-responses", &self.model);
        diagnostics.json("upstream.request", Some(&self.url()), &body);

        let response = self.http.post(self.url());
        let response = self
            .apply_auth(response)
            .json(&body)
            .send()
            .await
            .map_err(|err| ProviderError::internal(err.to_string()))?;

        let status = response.status();
        let headers = response.headers().clone();
        diagnostics.response_start(status.as_u16(), response.headers());
        let bytes = response
            .bytes()
            .await
            .map_err(|err| ProviderError::internal(err.to_string()))?;
        diagnostics.bytes("upstream.response_body", &bytes);
        if !status.is_success() {
            return Err(self.parse_failure(status, &headers, &bytes));
        }

        let parsed: Value = serde_json::from_slice(&bytes)
            .map_err(|err| ProviderError::internal(format!("invalid responses payload: {err}")))?;
        response_from_value(&parsed, UnifiedSourceProtocol::Responses)
            .map_err(ProviderError::internal)
    }

    async fn stream<'a>(
        &'a self,
        request: UnifiedRequest,
        _metadata: &M,
    ) -> Result<
        Pin<Box<dyn Stream<Item = Result<UnifiedEvent, ProviderError>> + Send + 'a>>,
        ProviderError,
    > {
        let mut body = request_to_value(&request, UnifiedSourceProtocol::Responses)
            .map_err(ProviderError::internal)?;
        body["model"] = json!(self.model);
        body["stream"] = json!(true);
        let diagnostics = UpstreamLog::new("openai-responses", &self.model);
        diagnostics.json("upstream.request", Some(&self.url()), &body);

        let response = timeout(
            Duration::from_secs(STREAM_FIRST_BYTE_TIMEOUT_SECS),
            self.apply_auth(self.http.post(self.url()))
                .json(&body)
                .send(),
        )
        .await
        .map_err(|_| ProviderError::internal("stream first byte timeout".to_string()))?
        .map_err(|err| ProviderError::internal(err.to_string()))?;

        let status = response.status();
        diagnostics.response_start(status.as_u16(), response.headers());
        if !status.is_success() {
            let headers = response.headers().clone();
            let bytes = response
                .bytes()
                .await
                .map_err(|err| ProviderError::internal(err.to_string()))?;
            diagnostics.bytes("upstream.response_body", &bytes);
            return Err(self.parse_failure(status, &headers, &bytes));
        }

        let byte_stream = response.bytes_stream();
        let output = try_stream! {
            let mut reader = SseDataReader::default();
            let mut state = StreamMapState::default();
            let mut stream = byte_stream;
            loop {
                let chunk = timeout(
                    Duration::from_secs(STREAM_IDLE_TIMEOUT_SECS),
                    stream.next(),
                )
                .await
                .map_err(|_| ProviderError::internal("stream idle timeout".to_string()))?;
                let Some(chunk) = chunk else { break };
                let chunk = chunk.map_err(|err| ProviderError::internal(err.to_string()))?;

                for data in reader.push(&chunk) {
                    diagnostics.bytes("upstream.stream_event", data.as_bytes());
                    if data == "[DONE]" {
                        return;
                    }
                    let event: Value = serde_json::from_str(&data)
                        .map_err(|err| {
                            ProviderError::internal(format!(
                                "invalid responses stream event: {err}"
                            ))
                        })?;
                    for unified in mapper::stream_event_from_value(&event, &mut state).map_err(ProviderError::internal)? {
                        yield unified;
                    }
                }
            }
        };
        Ok(Box::pin(output))
    }
}
